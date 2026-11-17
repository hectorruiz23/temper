package dev.temper.intervals

import dev.temper.api.PendingOperation
import dev.temper.api.TemperApi
import dev.temper.api.TemperJson
import dev.temper.api.WriteState
import dev.temper.config.StoredConfig
import dev.temper.model.CheckInEnvelope
import dev.temper.model.CheckInRequest
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

class RunnerReporter(
    private val api: TemperApi,
    private val scope: CoroutineScope,
) {
    private val mutableState = MutableStateFlow<WriteState<CheckInEnvelope>>(WriteState.Idle)
    val state: StateFlow<WriteState<CheckInEnvelope>> = mutableState.asStateFlow()

    private var operation: PendingOperation<CheckInEnvelope>? = null
    private var config: StoredConfig? = null
    private var completion: RunnerCompletion? = null

    fun report(config: StoredConfig?, completion: RunnerCompletion) {
        if (mutableState.value is WriteState.Sending) return
        if (operation?.state?.value is WriteState.Saved) return
        if (config == null) {
            mutableState.value = WriteState.UnknownOutcome("The server configuration is gone. The result is still in memory.")
            return
        }
        this.config = config
        this.completion = completion
        val existing = operation
        val next = when {
            existing == null -> newOperation(UUID.randomUUID().toString(), bodyFor(completion), config, completion)
            existing.state.value is WriteState.Rejected -> newOperation(existing.key, existing.body, config, completion)
            else -> existing
        }
        operation = next
        mutableState.value = WriteState.Sending
        scope.launch {
            if (next === existing) next.retry() else next.submit()
            mutableState.value = next.state.value
        }
    }

    fun retry() {
        val existing = operation ?: return
        when (existing.state.value) {
            is WriteState.Retryable, is WriteState.UnknownOutcome -> {
                mutableState.value = WriteState.Sending
                scope.launch {
                    existing.retry()
                    mutableState.value = existing.state.value
                }
            }
            is WriteState.Rejected -> {
                val currentConfig = config ?: return
                val currentCompletion = completion ?: return
                val next = newOperation(existing.key, existing.body, currentConfig, currentCompletion)
                operation = next
                mutableState.value = WriteState.Sending
                scope.launch {
                    next.submit()
                    mutableState.value = next.state.value
                }
            }
            else -> Unit
        }
    }

    fun reset() {
        operation = null
        config = null
        completion = null
        mutableState.value = WriteState.Idle
    }

    private fun newOperation(
        key: String,
        body: String,
        config: StoredConfig,
        completion: RunnerCompletion,
    ): PendingOperation<CheckInEnvelope> = PendingOperation(
        key = key,
        body = body,
        send = { requestKey, requestBody ->
            api.checkIn(config.endpoint, config.token, completion.sessionId, requestKey, requestBody)
        },
        decode = { response -> TemperJson.decodeFromString<CheckInEnvelope>(response) },
    )

    private fun bodyFor(completion: RunnerCompletion): String = TemperJson.encodeToString(
        CheckInRequest(
            expectedRevision = completion.expectedRevision,
            reportedCompletedAt = Instant.ofEpochMilli(completion.completedAtMs).toString(),
        ),
    )
}
