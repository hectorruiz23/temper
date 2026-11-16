package dev.temper

import dev.temper.api.TemperJson
import dev.temper.model.CheckInRequest
import dev.temper.model.LogSet
import dev.temper.model.LogSetsRequest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class JsonContractTest {
    @Test
    fun anAbsentNoteIsOmittedInsteadOfSentAsNull() {
        val body = TemperJson.encodeToString(
            LogSetsRequest(
                sessionId = "session-1",
                sets = listOf(LogSet(exerciseId = "goblet-squat", setIndex = 1, kg = 20.0, reps = 10)),
            ),
        )

        assertFalse(body.contains("note"))
        assertTrue(body.contains("\"session_id\":\"session-1\""))
    }

    @Test
    fun theReportedCheckInOmitsTheWatchWorkoutField() {
        val body = TemperJson.encodeToString(
            CheckInRequest(expectedRevision = 1, reportedCompletedAt = "2026-11-19T18:00:00Z"),
        )

        assertFalse(body.contains("watch_workout_id"))
        assertTrue(body.contains("\"expected_revision\":1"))
    }

    @Test
    fun theWatchCheckInOmitsTheReportedTimestamp() {
        val body = TemperJson.encodeToString(
            CheckInRequest(expectedRevision = 2, watchWorkoutId = "workout-1"),
        )

        assertFalse(body.contains("reported_completed_at"))
        assertTrue(body.contains("\"watch_workout_id\":\"workout-1\""))
    }
}
