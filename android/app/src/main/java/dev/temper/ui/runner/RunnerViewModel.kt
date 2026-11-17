package dev.temper.ui.runner

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.temper.AppContainer
import dev.temper.api.PendingOperation
import dev.temper.api.ProblemKind
import dev.temper.api.ReadOutcome
import dev.temper.api.TemperJson
import dev.temper.api.WriteState
import dev.temper.intervals.RunnerLaunch
import dev.temper.intervals.SystemWallClock
import dev.temper.model.CreateSessionRequest
import dev.temper.model.PlanSession
import dev.temper.model.SessionEnvelope
import dev.temper.model.SessionOpening
import dev.temper.model.decideSessionOpening
import dev.temper.runner.RunnerStep
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

data class RunnerContent(
    val target: String,
    val date: String,
    val templateId: String,
    val planVersionId: Long,
    val steps: List<RunnerStep>,
    val existingSessionId: String? = null,
    val existingRevision: Long = 0,
) {
    val distanceRequired: Boolean get() = steps.any { it.isDistance }
}

sealed interface RunnerUiState {
    data object Loading : RunnerUiState
    data object NeedsConfiguration : RunnerUiState
    data class Content(val content: RunnerContent) : RunnerUiState
    data object Empty : RunnerUiState
    data class Error(val message: String, val retryable: Boolean, val requiresConfiguration: Boolean = false) : RunnerUiState
}

class RunnerViewModel(
    private val container: AppContainer,
    private val dateValue: String,
    private val templateId: String,
    private val planVersionId: Long,
) : ViewModel() {
    private val mutableState = MutableStateFlow<RunnerUiState>(RunnerUiState.Loading)
    val state: StateFlow<RunnerUiState> = mutableState.asStateFlow()

    private val mutableCreateState = MutableStateFlow<WriteState<SessionEnvelope>>(WriteState.Idle)
    val createState: StateFlow<WriteState<SessionEnvelope>> = mutableCreateState.asStateFlow()

    val run = container.runnerController.snapshot
    val completion = container.runnerController.completion
    val report = container.runnerReporter.state

    private val sessionId = UUID.randomUUID().toString()
    private var operation: PendingOperation<SessionEnvelope>? = null

    init {
        val launch = container.runnerController.launch.value
        if (launch != null) {
            mutableState.value = RunnerUiState.Content(
                RunnerContent(
                    target = launch.target,
                    date = launch.date,
                    templateId = launch.templateId,
                    planVersionId = launch.planVersionId,
                    steps = launch.steps,
                ),
            )
        } else {
            load()
        }
    }

    fun load() {
        val config = container.configuration.value
        if (config == null) {
            mutableState.value = RunnerUiState.NeedsConfiguration
            return
        }
        mutableState.value = RunnerUiState.Loading
        viewModelScope.launch {
            when (val outcome = container.api.plan(config.endpoint, config.token, planVersionId)) {
                is ReadOutcome.Success -> {
                    val template = outcome.value.plan.sessions.firstOrNull { it.id == templateId }
                    if (template == null) {
                        mutableState.value = RunnerUiState.Empty
                        return@launch
                    }
                    val steps = mapSteps(template)
                    if (steps.isNullOrEmpty()) {
                        mutableState.value = RunnerUiState.Empty
                        return@launch
                    }
                    val week = when (val weekOutcome = container.api.week(config.endpoint, config.token, dateValue)) {
                        is ReadOutcome.Success -> weekOutcome.value
                        is ReadOutcome.Failure -> {
                            mutableState.value = RunnerUiState.Error(
                                message = weekOutcome.problem.message,
                                retryable = weekOutcome.problem.kind == ProblemKind.Retryable || weekOutcome.problem.kind == ProblemKind.Unknown,
                                requiresConfiguration = weekOutcome.problem.status == 401,
                            )
                            return@launch
                        }
                    }
                    val opening = decideSessionOpening(week.performed, dateValue, templateId) as? SessionOpening.Continue
                    mutableState.value = RunnerUiState.Content(
                        RunnerContent(
                            target = template.target.ifBlank { "Intervals" },
                            date = dateValue,
                            templateId = templateId,
                            planVersionId = planVersionId,
                            steps = steps,
                            existingSessionId = opening?.sessionId,
                            existingRevision = opening?.revision ?: 0,
                        ),
                    )
                }
                is ReadOutcome.Failure -> mutableState.value = RunnerUiState.Error(
                    message = outcome.problem.message,
                    retryable = outcome.problem.kind == ProblemKind.Retryable || outcome.problem.kind == ProblemKind.Unknown,
                    requiresConfiguration = outcome.problem.status == 401,
                )
            }
        }
    }

    fun start() {
        val content = (mutableState.value as? RunnerUiState.Content)?.content ?: return
        val config = container.configuration.value
        if (config == null) {
            mutableState.value = RunnerUiState.NeedsConfiguration
            return
        }
        if (mutableCreateState.value is WriteState.Sending) return
        if (container.runnerController.snapshot.value != null) return
        val existingSessionId = content.existingSessionId
        if (existingSessionId != null) {
            prepareRun(content, existingSessionId, content.existingRevision)
            return
        }
        viewModelScope.launch {
            val existing = operation
            val current = existing?.state?.value
            val reuse = existing != null && (current is WriteState.Retryable || current is WriteState.UnknownOutcome)
            val next = if (reuse) {
                existing
            } else {
                PendingOperation(
                    key = UUID.randomUUID().toString(),
                    body = TemperJson.encodeToString(
                        CreateSessionRequest(
                            id = sessionId,
                            templateId = content.templateId,
                            date = content.date,
                            discipline = "intervals",
                            startedAt = Instant.ofEpochMilli(SystemWallClock.nowMs()).toString(),
                        ),
                    ),
                    send = { key, body -> container.api.createSession(config.endpoint, config.token, key, body) },
                    decode = { body -> TemperJson.decodeFromString<SessionEnvelope>(body) },
                )
            }
            operation = next
            mutableCreateState.value = WriteState.Sending
            if (reuse) next.retry() else next.submit()
            mutableCreateState.value = next.state.value
            val saved = next.state.value as? WriteState.Saved<SessionEnvelope> ?: return@launch
            val session = saved.value.session
            prepareRun(content, session.id, session.revision)
        }
    }

    private fun prepareRun(content: RunnerContent, sessionId: String, revision: Long) {
        container.runnerController.prepare(
            RunnerLaunch(
                target = content.target,
                date = content.date,
                templateId = content.templateId,
                sessionId = sessionId,
                sessionRevision = revision,
                startedAtMs = SystemWallClock.nowMs(),
                steps = content.steps,
                planVersionId = content.planVersionId,
            ),
        )
        container.runnerReporter.reset()
        container.startRunnerService()
    }

    fun retryStart() {
        val existing = operation ?: return
        val current = existing.state.value
        if (current !is WriteState.Retryable && current !is WriteState.UnknownOutcome) return
        start()
    }

    fun retryReport() {
        container.runnerReporter.retry()
    }

    fun reportNow() {
        val completion = container.runnerController.completion.value ?: return
        container.runnerReporter.report(container.configuration.value, completion)
    }

    fun closeFinished() {
        if (container.runnerReporter.state.value is WriteState.Saved) {
            container.runnerController.reset()
            container.runnerReporter.reset()
        }
    }

    fun discardRun() {
        container.discardRunner()
        container.runnerController.reset()
        container.runnerReporter.reset()
    }

    private fun mapSteps(template: PlanSession): List<RunnerStep>? {
        val mapped = mutableListOf<RunnerStep>()
        for (step in template.intervalSteps.sortedBy { it.position }) {
            val duration = step.durationMs?.toLong()
            val distance = step.distanceM
            when {
                duration != null && duration > 0L && distance == null ->
                    mapped += RunnerStep(step.position, step.label, durationMs = duration)
                distance != null && distance > 0.0 && duration == null ->
                    mapped += RunnerStep(step.position, step.label, distanceM = distance)
                else -> return null
            }
        }
        return mapped
    }
}
