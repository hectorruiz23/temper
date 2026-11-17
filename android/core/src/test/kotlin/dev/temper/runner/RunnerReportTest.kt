package dev.temper.runner

import dev.temper.api.PendingOperation
import dev.temper.api.SendOutcome
import dev.temper.api.TemperJson
import dev.temper.api.WriteState
import dev.temper.model.CheckInEnvelope
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class RunnerReportTest {
    @Test
    fun aLostAnswerRetriesWithTheSameKeyAndCommitsOnce() = runBlocking {
        val server = IdempotentCheckInServer()
        val operation = PendingOperation(
            key = "report-key",
            body = """{"expected_revision":1,"reported_completed_at":"2026-12-20T10:30:00Z"}""",
            send = { key, body -> server.send(key, body) },
            decode = { body -> TemperJson.decodeFromString<CheckInEnvelope>(body) },
        )

        operation.submit()
        assertTrue(operation.state.value is WriteState.UnknownOutcome)

        operation.retry()
        assertTrue(operation.state.value is WriteState.Saved)
        assertEquals(2, server.requests)
        assertEquals(1, server.commits)
        assertEquals(listOf("report-key", "report-key"), server.keys)
    }

    private class IdempotentCheckInServer {
        var requests = 0
            private set
        var commits = 0
            private set
        val keys = mutableListOf<String>()
        private val responses = mutableMapOf<String, String>()

        fun send(key: String, body: String): SendOutcome {
            requests += 1
            keys += key
            val response = responses.getOrPut(key) {
                commits += 1
                """{"session":{"id":"s1","revision":2},"effective_evidence":{"source":"reported"}}"""
            }
            return if (requests == 1) {
                SendOutcome.Unknown("The server committed and the answer was lost.")
            } else {
                SendOutcome.Success(200, response)
            }
        }
    }
}
