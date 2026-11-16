package dev.temper.api

import java.security.MessageDigest
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

class PendingOperation<T>(
    val key: String = UUID.randomUUID().toString(),
    val body: String,
    private val send: suspend (key: String, body: String) -> SendOutcome,
    private val decode: (body: String) -> T,
) {
    private val mutableState = MutableStateFlow<WriteState<T>>(WriteState.Idle)
    val state: StateFlow<WriteState<T>> = mutableState.asStateFlow()
    val payloadHash: String = sha256(body)

    suspend fun submit() {
        if (mutableState.value is WriteState.Sending) return
        execute()
    }

    suspend fun retry() {
        when (mutableState.value) {
            is WriteState.Retryable, is WriteState.UnknownOutcome -> execute()
            else -> Unit
        }
    }

    fun edited(newBody: String): PendingOperation<T> =
        PendingOperation(key = UUID.randomUUID().toString(), body = newBody, send = send, decode = decode)

    private suspend fun execute() {
        mutableState.value = WriteState.Sending
        mutableState.value = when (val outcome = send(key, body)) {
            is SendOutcome.Success -> {
                runCatching { decode(outcome.body) }.fold(
                    onSuccess = { WriteState.Saved(it) },
                    onFailure = { WriteState.UnknownOutcome("The server answered, but the reply could not be read.") },
                )
            }
            is SendOutcome.Retryable -> WriteState.Retryable(outcome.reason)
            is SendOutcome.Unknown -> WriteState.UnknownOutcome(outcome.reason)
            is SendOutcome.Rejected -> WriteState.Rejected(outcome.status, outcome.code, outcome.message)
        }
    }

    private fun sha256(value: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(value.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { byte -> "%02x".format(byte) }
    }
}
