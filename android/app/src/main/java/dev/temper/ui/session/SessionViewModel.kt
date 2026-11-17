package dev.temper.ui.session

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.temper.AppContainer
import dev.temper.api.PendingOperation
import dev.temper.api.ProblemKind
import dev.temper.api.ReadOutcome
import dev.temper.api.TemperJson
import dev.temper.api.WriteState
import dev.temper.model.CreateSessionRequest
import dev.temper.model.PlanExercise
import dev.temper.model.PlanSession
import dev.temper.model.SessionEnvelope
import dev.temper.model.SessionOpening
import dev.temper.model.decideSessionOpening
import java.time.Instant
import java.time.LocalDate
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

data class SessionExerciseView(
    val exerciseId: String,
    val name: String,
    val target: String,
    val source: PlanExercise,
)

data class SessionContent(
    val date: LocalDate,
    val planVersion: Long,
    val discipline: String,
    val target: String,
    val exercises: List<SessionExerciseView>,
    val existingSessionId: String? = null,
    val existingRevision: Long = 0,
)

sealed interface SessionUiState {
    data object Loading : SessionUiState
    data object NeedsConfiguration : SessionUiState
    data class Content(val content: SessionContent) : SessionUiState
    data class Empty(val message: String) : SessionUiState
    data class Error(val message: String, val retryable: Boolean, val requiresConfiguration: Boolean = false) : SessionUiState
}

class SessionViewModel(
    private val container: AppContainer,
    private val dateValue: String,
    private val templateId: String,
) : ViewModel() {
    private val mutableState = MutableStateFlow<SessionUiState>(SessionUiState.Loading)
    val state: StateFlow<SessionUiState> = mutableState.asStateFlow()

    private val mutableWriteState = MutableStateFlow<WriteState<SessionEnvelope>>(WriteState.Idle)
    val writeState: StateFlow<WriteState<SessionEnvelope>> = mutableWriteState.asStateFlow()

    private var operation: PendingOperation<SessionEnvelope>? = null

    init {
        load()
    }

    fun load() {
        val config = container.configuration.value
        if (config == null) {
            mutableState.value = SessionUiState.NeedsConfiguration
            return
        }
        val date = runCatching { LocalDate.parse(dateValue) }.getOrNull()
        if (date == null) {
            mutableState.value = SessionUiState.Error("The session date is not valid.", false)
            return
        }
        mutableState.value = SessionUiState.Loading
        viewModelScope.launch {
            val week = when (val outcome = container.api.week(config.endpoint, config.token, date.toString())) {
                is ReadOutcome.Success -> outcome.value
                is ReadOutcome.Failure -> {
                    mutableState.value = error(outcome.problem.message, outcome.problem.kind, outcome.problem.status)
                    return@launch
                }
            }
            val plan = when (val outcome = container.api.currentPlan(config.endpoint, config.token)) {
                is ReadOutcome.Success -> outcome.value.plan
                is ReadOutcome.Failure -> {
                    mutableState.value = error(outcome.problem.message, outcome.problem.kind, outcome.problem.status)
                    return@launch
                }
            }
            val exerciseList = when (val outcome = container.api.exercises(config.endpoint, config.token)) {
                is ReadOutcome.Success -> outcome.value.exercises
                is ReadOutcome.Failure -> {
                    mutableState.value = error(outcome.problem.message, outcome.problem.kind, outcome.problem.status)
                    return@launch
                }
            }
            val template = plan.sessions.firstOrNull { it.id == templateId }
            if (template == null) {
                mutableState.value = SessionUiState.Empty("This plan version has no session with that id.")
                return@launch
            }
            val planned = week.planned.firstOrNull { it.templateId == templateId && it.date == dateValue }
            val opening = decideSessionOpening(week.performed, dateValue, templateId)
            val names = exerciseList.associate { it.id to it.name }
            val views = template.exercises.sortedBy { it.position }.map { exercise ->
                SessionExerciseView(
                    exerciseId = exercise.exerciseId,
                    name = names[exercise.exerciseId] ?: exercise.exerciseId,
                    target = targetLabel(exercise),
                    source = exercise,
                )
            }
            mutableState.value = SessionUiState.Content(
                SessionContent(
                    date = date,
                    planVersion = planned?.planVersionId ?: plan.id,
                    discipline = planned?.discipline ?: template.discipline,
                    target = planned?.target?.ifBlank { template.target } ?: template.target,
                    exercises = views,
                    existingSessionId = (opening as? SessionOpening.Continue)?.sessionId,
                    existingRevision = (opening as? SessionOpening.Continue)?.revision ?: 0,
                ),
            )
        }
    }

    fun start() {
        val content = (mutableState.value as? SessionUiState.Content)?.content ?: return
        val config = container.configuration.value ?: return
        if (operation?.state?.value is WriteState.Sending) return
        viewModelScope.launch {
            val existing = operation
            val current = existing?.state?.value
            val next = if (existing != null && (current is WriteState.Retryable || current is WriteState.UnknownOutcome)) {
                existing
            } else {
                PendingOperation(
                    key = UUID.randomUUID().toString(),
                    body = TemperJson.encodeToString(
                        CreateSessionRequest(
                            id = UUID.randomUUID().toString(),
                            templateId = templateId,
                            date = content.date.toString(),
                            discipline = content.discipline,
                            startedAt = Instant.now().toString(),
                        ),
                    ),
                    send = { key, body -> container.api.createSession(config.endpoint, config.token, key, body) },
                    decode = { body -> TemperJson.decodeFromString<SessionEnvelope>(body) },
                )
            }
            operation = next
            mutableWriteState.value = WriteState.Sending
            if (next === existing) next.retry() else next.submit()
            mutableWriteState.value = next.state.value
        }
    }

    fun retry() {
        val existing = operation ?: return
        val current = existing.state.value
        if (current !is WriteState.Retryable && current !is WriteState.UnknownOutcome) return
        viewModelScope.launch {
            mutableWriteState.value = WriteState.Sending
            existing.retry()
            mutableWriteState.value = existing.state.value
        }
    }

    private fun error(message: String, kind: ProblemKind, status: Int?): SessionUiState =
        SessionUiState.Error(
            message = message,
            retryable = kind == ProblemKind.Retryable || kind == ProblemKind.Unknown,
            requiresConfiguration = status == 401,
        )

    private fun targetLabel(exercise: PlanExercise): String {
        val sets = exercise.targetSets
        val reps = when {
            exercise.repsMin != null && exercise.repsMax != null && exercise.repsMin != exercise.repsMax ->
                "${exercise.repsMin}–${exercise.repsMax}"
            exercise.repsMin != null -> exercise.repsMin.toString()
            exercise.repsMax != null -> exercise.repsMax.toString()
            else -> "?"
        }
        val base = if (sets != null) "$sets×$reps" else "$reps reps"
        val kg = exercise.targetKg?.let { " · ${it.toString().removeSuffix(".0")} kg" } ?: ""
        val tempo = exercise.tempo?.let { " · tempo $it" } ?: ""
        return base + kg + tempo
    }
}
