package dev.temper

import dev.temper.model.Evidence
import dev.temper.model.PerformedSession
import dev.temper.model.SessionOpening
import dev.temper.model.decideSessionOpening
import org.junit.Assert.assertEquals
import org.junit.Test

class SessionOpeningTest {
    private val performed = listOf(
        PerformedSession(
            id = "session-1",
            date = "2026-11-17",
            templateId = "s1",
            discipline = "strength",
            revision = 2,
            complete = true,
            evidence = Evidence(source = "reported", reportedCompletedAt = "2026-11-17T09:00:00Z"),
        ),
    )

    @Test
    fun aMatchingSessionOnTheDayIsContinued() {
        assertEquals(
            SessionOpening.Continue("session-1", 2),
            decideSessionOpening(performed, "2026-11-17", "s1"),
        )
    }

    @Test
    fun aMatchingSessionOnAnotherDateCreatesOne() {
        assertEquals(
            SessionOpening.Create,
            decideSessionOpening(performed, "2026-11-18", "s1"),
        )
    }

    @Test
    fun noPerformedSessionCreatesOne() {
        assertEquals(
            SessionOpening.Create,
            decideSessionOpening(performed, "2026-11-17", "s2"),
        )
        assertEquals(
            SessionOpening.Create,
            decideSessionOpening(emptyList(), "2026-11-17", "s1"),
        )
    }

    @Test
    fun aPlanEntryWithoutAnIdMatchesOnTheDate() {
        assertEquals(
            SessionOpening.Continue("session-1", 2),
            decideSessionOpening(performed, "2026-11-17", ""),
        )
    }
}
