package dev.temper.api

enum class ProblemKind {
    Retryable,
    Unknown,
    Rejected,
    Configuration,
}

data class ApiProblem(
    val kind: ProblemKind,
    val message: String,
    val code: String? = null,
    val status: Int? = null,
)

sealed interface ReadOutcome<out T> {
    data class Success<T>(val value: T) : ReadOutcome<T>
    data class Failure(val problem: ApiProblem) : ReadOutcome<Nothing>
}

sealed interface SendOutcome {
    data class Success(val status: Int, val body: String) : SendOutcome
    data class Retryable(val reason: String) : SendOutcome
    data class Unknown(val reason: String) : SendOutcome
    data class Rejected(val status: Int, val code: String?, val message: String) : SendOutcome
}
