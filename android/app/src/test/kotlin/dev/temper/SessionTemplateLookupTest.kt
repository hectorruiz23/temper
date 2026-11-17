package dev.temper

import dev.temper.model.PlanExercise
import dev.temper.model.PlanSession
import dev.temper.ui.sets.SessionTemplateLookup
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionTemplateLookupTest {
    private val strength = PlanSession(
        id = "fuerza-a",
        discipline = "strength",
        exercises = listOf(PlanExercise(position = 0, exerciseId = "goblet-squat", targetSets = 3)),
    )

    @Test
    fun theSessionIsFoundInTheVersionItWasPlannedFrom() {
        assertEquals(strength, SessionTemplateLookup.inPlan(listOf(strength), "fuerza-a"))
    }

    @Test
    fun aTemplateMissingFromTheVersionInForceIsNotInvented() {
        assertNull(SessionTemplateLookup.inPlan(listOf(strength), "fuerza-b"))
    }

    @Test
    fun theMissingTemplateIsNamedInTheMessage() {
        val message = SessionTemplateLookup.missingMessage("fuerza-b", "2026-11-17")

        assertTrue(message.contains("fuerza-b"))
        assertTrue(message.contains("2026-11-17"))
        assertTrue(message.contains("discard"))
    }
}
