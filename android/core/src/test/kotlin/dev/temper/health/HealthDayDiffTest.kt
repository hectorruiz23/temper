package dev.temper.health

import dev.temper.model.HealthDay
import java.time.LocalDate
import java.time.ZoneId
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class HealthDayDiffTest {
    private val zone = ZoneId.of("Europe/Madrid")
    private val granted = setOf(HealthPermission.Steps)
    private val day = LocalDate.of(2026, 11, 17)

    @Test
    fun aResyncWithTheSameTotalAndSourceIsCurrent() {
        val existing = HealthDay(
            date = day.toString(),
            timezone = zone.id,
            revision = 21,
            steps = 11_000,
            stepsSource = "watch",
        )

        assertTrue(HealthDayDiff.sameDay(existing, snapshot(steps = 11_000, source = "watch"), granted))
    }

    @Test
    fun aSecondSyncWithMoreStepsIsUploaded() {
        val existing = HealthDay(
            date = day.toString(),
            timezone = zone.id,
            revision = 4,
            steps = 24_072,
            stepsSource = "watch",
        )

        assertFalse(HealthDayDiff.sameDay(existing, snapshot(steps = 24_400, source = "watch"), granted))
    }

    @Test
    fun aChangedSourceIsUploadedSoTheServerReplacesTheCount() {
        val existing = HealthDay(
            date = day.toString(),
            timezone = zone.id,
            revision = 4,
            steps = 24_072,
            stepsSource = "phone",
        )

        assertFalse(HealthDayDiff.sameDay(existing, snapshot(steps = 11_000, source = "watch"), granted))
    }

    @Test
    fun anUnknownSourceOnTheServerIsUploadedOnce() {
        val existing = HealthDay(
            date = day.toString(),
            timezone = zone.id,
            revision = 2,
            steps = 11_000,
        )

        assertFalse(HealthDayDiff.sameDay(existing, snapshot(steps = 11_000, source = "watch"), granted))
    }

    private fun snapshot(steps: Long, source: String): HealthDaySnapshot = HealthDaySnapshot(
        date = day,
        timezone = zone,
        steps = steps,
        stepsSource = source,
    )
}
