package dev.temper.api

import dev.temper.model.ApiErrorEnvelope
import dev.temper.model.CheckInEnvelope
import dev.temper.model.ExerciseListResponse
import dev.temper.model.HealthDayEnvelope
import dev.temper.model.HealthResponse
import dev.temper.model.PlanEnvelope
import dev.temper.model.SessionEnvelope
import dev.temper.model.SetsEnvelope
import dev.temper.model.WeekResponse
import io.ktor.client.HttpClient
import io.ktor.client.engine.cio.CIO
import io.ktor.client.network.sockets.ConnectTimeoutException
import io.ktor.client.network.sockets.SocketTimeoutException
import io.ktor.client.plugins.HttpRequestTimeoutException
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.plugins.contentnegotiation.ContentNegotiation
import io.ktor.client.request.get
import io.ktor.client.request.header
import io.ktor.client.request.post
import io.ktor.client.request.setBody
import io.ktor.client.statement.HttpResponse
import io.ktor.client.statement.bodyAsText
import io.ktor.http.ContentType
import io.ktor.http.HttpStatusCode
import io.ktor.http.contentType
import io.ktor.serialization.kotlinx.json.json
import java.io.IOException
import java.net.ConnectException
import java.net.UnknownHostException
import java.nio.channels.UnresolvedAddressException
import javax.net.ssl.SSLException
import kotlinx.serialization.json.Json

class TemperApi {
    private val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        encodeDefaults = false
    }

    private val client = HttpClient(CIO) {
        expectSuccess = false
        install(ContentNegotiation) {
            json(json)
        }
        install(HttpTimeout) {
            connectTimeoutMillis = 5_000
            requestTimeoutMillis = 10_000
            socketTimeoutMillis = 10_000
        }
    }

    suspend fun health(endpoint: String): ReadOutcome<HealthResponse> =
        read(endpoint, "/api/health", null)

    suspend fun week(endpoint: String, token: String, date: String): ReadOutcome<WeekResponse> =
        read(endpoint, "/api/week/$date", token)

    suspend fun currentPlan(endpoint: String, token: String): ReadOutcome<PlanEnvelope> =
        read(endpoint, "/api/plans/current", token)

    suspend fun exercises(endpoint: String, token: String): ReadOutcome<ExerciseListResponse> =
        read(endpoint, "/api/exercises", token)

    suspend fun day(endpoint: String, token: String, date: String): ReadOutcome<HealthDayEnvelope> =
        read(endpoint, "/api/day/$date", token)

    suspend fun uploadDay(endpoint: String, token: String, key: String, body: String): SendOutcome =
        send(endpoint, token, "/api/day", key, body)

    suspend fun createSession(endpoint: String, token: String, key: String, body: String): SendOutcome =
        send(endpoint, token, "/api/sessions", key, body)

    suspend fun logSets(endpoint: String, token: String, key: String, body: String): SendOutcome =
        send(endpoint, token, "/api/sets", key, body)

    suspend fun checkIn(endpoint: String, token: String, sessionId: String, key: String, body: String): SendOutcome =
        send(endpoint, token, "/api/sessions/$sessionId/check-in", key, body)

    fun close() {
        client.close()
    }

    private suspend inline fun <reified T> read(endpoint: String, path: String, token: String?): ReadOutcome<T> {
        val url = endpoint.trimEnd('/') + path
        val response = try {
            client.get(url) {
                if (token != null) header("X-Temper-Token", token)
            }
        } catch (error: Throwable) {
            return ReadOutcome.Failure(problemFor(error, reading = true))
        }
        return readResponse(response)
    }

    private suspend inline fun <reified T> readResponse(response: HttpResponse): ReadOutcome<T> {
        val status = response.status
        val text = try {
            response.bodyAsText()
        } catch (error: Throwable) {
            return ReadOutcome.Failure(problemFor(error, reading = true))
        }
        if (status.value !in 200..299) {
            return ReadOutcome.Failure(problemFromStatus(status.value, text))
        }
        return try {
            ReadOutcome.Success(json.decodeFromString<T>(text))
        } catch (error: Throwable) {
            ReadOutcome.Failure(
                ApiProblem(
                    kind = ProblemKind.Unknown,
                    message = "The server answered, but the reply could not be read.",
                    status = status.value,
                ),
            )
        }
    }

    private suspend fun send(endpoint: String, token: String, path: String, key: String, body: String): SendOutcome {
        val url = endpoint.trimEnd('/') + path
        val response = try {
            client.post(url) {
                header("X-Temper-Token", token)
                header("Idempotency-Key", key)
                contentType(ContentType.Application.Json)
                setBody(body)
            }
        } catch (error: Throwable) {
            val problem = problemFor(error, reading = false)
            return when (problem.kind) {
                ProblemKind.Retryable -> SendOutcome.Retryable(problem.message)
                else -> SendOutcome.Unknown(problem.message)
            }
        }
        val status = response.status
        val text = try {
            response.bodyAsText()
        } catch (error: Throwable) {
            val problem = problemFor(error, reading = false)
            return when (problem.kind) {
                ProblemKind.Retryable -> SendOutcome.Retryable(problem.message)
                else -> SendOutcome.Unknown(problem.message)
            }
        }
        if (status.value in 200..299) return SendOutcome.Success(status.value, text)
        if (status.value == HttpStatusCode.ServiceUnavailable.value) {
            return SendOutcome.Retryable("The server is unavailable right now.")
        }
        val problem = problemFromStatus(status.value, text)
        return SendOutcome.Rejected(status.value, problem.code, problem.message)
    }

    private fun problemFromStatus(status: Int, body: String): ApiProblem {
        val parsed = runCatching { json.decodeFromString<ApiErrorEnvelope>(body) }.getOrNull()
        val message = parsed?.error?.message?.takeIf { it.isNotBlank() }
            ?: "The server rejected the request (HTTP $status)."
        val kind = if (status == HttpStatusCode.ServiceUnavailable.value) ProblemKind.Retryable else ProblemKind.Rejected
        return ApiProblem(kind, message, parsed?.error?.code, status)
    }

    private fun problemFor(error: Throwable, reading: Boolean): ApiProblem {
        val chain = generateSequence(error) { it.cause }.toList()
        val retryable = chain.any {
            it is ConnectTimeoutException ||
                it is ConnectException ||
                it is UnknownHostException ||
                it is UnresolvedAddressException ||
                it is SSLException
        }
        if (retryable) {
            return ApiProblem(ProblemKind.Retryable, "The request could not reach the server.")
        }
        val timeout = chain.any {
            it is SocketTimeoutException || it is HttpRequestTimeoutException
        }
        if (timeout) {
            return ApiProblem(
                ProblemKind.Unknown,
                if (reading) "The server did not answer before the request timed out." else "The request left the phone, but the server did not answer. It may have been saved.",
            )
        }
        if (chain.any { it is IOException }) {
            return ApiProblem(
                ProblemKind.Unknown,
                if (reading) "The connection closed before the server answered." else "The request left the phone, but the connection closed before the server answered. It may have been saved.",
            )
        }
        return ApiProblem(ProblemKind.Unknown, "The request ended without an answer.")
    }
}
