package dev.temper

import dev.temper.api.PendingOperation
import dev.temper.api.SendOutcome
import dev.temper.api.WriteState
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PendingOperationTest {
    @Test
    fun retryReusesTheSameKeyAndTheSameBytes() = runTest {
        val keys = mutableListOf<String>()
        val bodies = mutableListOf<String>()
        var attempts = 0
        val operation = PendingOperation(
            key = "key-one",
            body = """{"session_id":"s1","sets":[]}""",
            send = { key, body ->
                keys += key
                bodies += body
                attempts += 1
                if (attempts == 1) SendOutcome.Retryable("No network") else SendOutcome.Success(201, """{"sets":[],"echo":{"session_id":"s1","sets":[]}}""")
            },
            decode = { json.decodeFromString<SetsPayload>(it) },
        )

        operation.submit()
        assertTrue(operation.state.value is WriteState.Retryable)
        operation.retry()

        assertEquals(listOf("key-one", "key-one"), keys)
        assertEquals(listOf(operation.body, operation.body), bodies)
        assertTrue(operation.state.value is WriteState.Saved)
    }

    @Test
    fun unknownOutcomeRetriesWithTheSameKey() = runTest {
        val keys = mutableListOf<String>()
        var attempts = 0
        val operation = PendingOperation(
            key = "key-two",
            body = """{"session_id":"s2","sets":[]}""",
            send = { key, _ ->
                keys += key
                attempts += 1
                if (attempts == 1) SendOutcome.Unknown("No answer") else SendOutcome.Success(201, """{"sets":[],"echo":{"session_id":"s2","sets":[]}}""")
            },
            decode = { json.decodeFromString<SetsPayload>(it) },
        )

        operation.submit()
        assertTrue(operation.state.value is WriteState.UnknownOutcome)
        operation.retry()

        assertEquals(listOf("key-two", "key-two"), keys)
        assertTrue(operation.state.value is WriteState.Saved)
    }

    @Test
    fun editingCreatesANewKeyAndANewPayloadHash() {
        val operation = PendingOperation(
            key = "key-three",
            body = """{"kg":20}""",
            send = { _, _ -> SendOutcome.Success(201, "{}") },
            decode = { it },
        )

        val edited = operation.edited("""{"kg":22.5}""")

        assertNotEquals(operation.key, edited.key)
        assertNotEquals(operation.payloadHash, edited.payloadHash)
    }

    @Test
    fun submitWhileSendingDoesNotSendTwice() = runTest {
        var calls = 0
        val started = kotlinx.coroutines.CompletableDeferred<Unit>()
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        val operation = PendingOperation(
            key = "key-four",
            body = """{"kg":20}""",
            send = { _, _ ->
                calls += 1
                started.complete(Unit)
                release.await()
                SendOutcome.Success(201, "{}")
            },
            decode = { it },
        )

        val first = launch { operation.submit() }
        started.await()
        operation.submit()
        release.complete(Unit)
        first.join()

        assertEquals(1, calls)
    }

    private val json = Json { ignoreUnknownKeys = true }

    @kotlinx.serialization.Serializable
    private data class SetsPayload(
        val sets: List<String> = emptyList(),
        val echo: EchoPayload = EchoPayload(),
    )

    @kotlinx.serialization.Serializable
    private data class EchoPayload(
        @kotlinx.serialization.SerialName("session_id") val sessionId: String = "",
        val sets: List<String> = emptyList(),
    )
}
