package dev.temper.ui.result

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.temper.AppContainer
import dev.temper.api.PendingOperation
import dev.temper.api.ProblemKind
import dev.temper.api.ReadOutcome
import dev.temper.api.TemperJson
import dev.temper.api.WriteState
import dev.temper.model.CheckInEnvelope
import dev.temper.model.CheckInRequest
import java.time.Instant
import java.time.LocalDate
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

sealed interface ResultUiState {
    data object Loading : ResultUiState
    data object NeedsConfiguration : ResultUiState
    data class Content(val watchWorkoutId: String?) : ResultUiState
    data class Error(val message: String, val retryable: Boolean, val requiresConfiguration: Boolean = false) : ResultUiState
}

class ResultViewModel(
    private val container: AppContainer,
    private val sessionId: String,
    private val dateValue: String,
    private val revision: Long,
) : ViewModel() {
    private val mutableState = MutableStateFlow<ResultUiState>(ResultUiState.Loading)
    val state: StateFlow<ResultUiState> = mutableState.asStateFlow()

    private val mutableWriteState = MutableStateFlow<WriteState<CheckInEnvelope>>(WriteState.Idle)
    val writeState: StateFlow<WriteState<CheckInEnvelope>> = mutableWriteState.asStateFlow()

    private var operation: PendingOperation<CheckInEnvelope>? = null
    private var operationBody: String? = null

    init {
        load()
    }

    fun load() {
        val config = container.configuration.value
        if (config == null) {
            mutableState.value = ResultUiState.NeedsConfiguration
            return
        }
        val date = runCatching { LocalDate.parse(dateValue) }.getOrNull()
        if (date == null) {
            mutableState.value = ResultUiState.Error("The session date is not valid.", false)
            return
        }
        mutableState.value = ResultUiState.Loading
        viewModelScope.launch {
            when (val outcome = container.api.week(config.endpoint, config.token, date.toString())) {
                is ReadOutcome.Success -> {
                    val watch = outcome.value.performed.firstOrNull {
                        it.date == dateValue && !it.watchWorkoutId.isNullOrBlank()
                    }?.watchWorkoutId
                    mutableState.value = ResultUiState.Content(watch)
                }
                is ReadOutcome.Failure -> mutableState.value = ResultUiState.Error(
                    message = outcome.problem.message,
                    retryable = outcome.problem.kind == ProblemKind.Retryable || outcome.problem.kind == ProblemKind.Unknown,
                    requiresConfiguration = outcome.problem.status == 401,
                )
            }
        }
    }

    fun markComplete() {
        val config = container.configuration.value ?: return
        if (mutableWriteState.value is WriteState.Sending) return
        submit(
            config = config,
            body = TemperJson.encodeToString(
                CheckInRequest(
                    expectedRevision = revision,
                    reportedCompletedAt = Instant.now().toString(),
                ),
            ),
        )
    }

    fun linkWatch() {
        val config = container.configuration.value ?: return
        val watch = (mutableState.value as? ResultUiState.Content)?.watchWorkoutId ?: return
        if (mutableWriteState.value is WriteState.Sending) return
        submit(
            config = config,
            body = TemperJson.encodeToString(
                CheckInRequest(
                    expectedRevision = revision,
                    watchWorkoutId = watch,
                ),
            ),
        )
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

    private fun submit(config: dev.temper.config.StoredConfig, body: String) {
        val existing = operation
        val current = existing?.state?.value
        val reuse = existing != null &&
            operationBody == body &&
            (current is WriteState.Retryable || current is WriteState.UnknownOutcome)
        val next = if (reuse) existing else PendingOperation(
            key = UUID.randomUUID().toString(),
            body = body,
            send = { key, payload ->
                container.api.checkIn(config.endpoint, config.token, sessionId, key, payload)
            },
            decode = { payload -> TemperJson.decodeFromString<CheckInEnvelope>(payload) },
        )
        operation = next
        operationBody = body
        viewModelScope.launch {
            mutableWriteState.value = WriteState.Sending
            if (reuse) next.retry() else next.submit()
            mutableWriteState.value = next.state.value
        }
    }
}
