package dev.temper.api

sealed interface WriteState<out T> {
    data object Idle : WriteState<Nothing>
    data object Sending : WriteState<Nothing>
    data class Saved<T>(val value: T) : WriteState<T>
    data class Retryable(val reason: String) : WriteState<Nothing>
    data class UnknownOutcome(val reason: String) : WriteState<Nothing>
    data class Rejected(val status: Int, val code: String?, val message: String) : WriteState<Nothing>
}
