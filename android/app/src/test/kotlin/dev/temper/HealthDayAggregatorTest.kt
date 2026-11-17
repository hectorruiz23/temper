package dev.temper

import dev.temper.api.TemperJson
import dev.temper.health.ExerciseSample
import dev.temper.health.HealthDayAggregator
import dev.temper.health.HealthPermission
import dev.temper.health.HealthRecords
import dev.temper.health.HrvRmssdSample
import dev.temper.health.SleepSample
import dev.temper.health.SleepStageSample
import dev.temper.health.StepsSample
import dev.temper.health.healthDayKey
import dev.temper.model.DayMetrics
import dev.temper.model.DayUploadRequest
import dev.temper.model.DayWorkout
import dev.temper.model.HealthSleepStage
import java.time.Duration
import java.time.LocalDate
import java.time.ZoneId
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HealthDayAggregatorTest {
    private val zone = ZoneId.of("Europe/Madrid")

    @Test
    fun missingHrvAndPartialSleepStagesStayAbsent() {
        val day = LocalDate.of(2026, 11, 16)
        val start = day.minusDays(1).atTime(22, 0).atZone(zone).toInstant()
        val end = day.atTime(6, 0).atZone(zone).toInstant()
        val midnight = day.atStartOfDay(zone).toInstant()
        val deepEnd = day.atTime(2, 0).atZone(zone).toInstant()
        val lightEnd = day.atTime(4, 0).atZone(zone).toInstant()

        val records = HealthRecords(
            sleep = listOf(
                SleepSample(
                    id = "sleep-1",
                    start = start,
                    end = end,
                    stages = listOf(
                        SleepStageSample("deep", midnight, deepEnd),
                        SleepStageSample("light", deepEnd, lightEnd),
                    ),
                ),
            ),
            steps = listOf(
                StepsSample(
                    id = "steps-1",
                    start = day.atTime(10, 0).atZone(zone).toInstant(),
                    end = day.atTime(11, 0).atZone(zone).toInstant(),
                    count = 0,
                ),
            ),
        )

        val snapshot = HealthDayAggregator.aggregate(records, day, zone)

        assertEquals(Duration.ofHours(6).toMillis(), snapshot.sleepMs)
        assertEquals(0L, snapshot.steps)
        assertNull(snapshot.restingHrBpm)
        assertNull(snapshot.hrvRmssdMs)
        assertEquals(listOf("deep", "light"), snapshot.sleepStages.map { it.stage })
        assertEquals(Duration.ofHours(2).toMillis(), snapshot.sleepStages.first().durationMs)
        assertFalse(snapshot.sleepStages.any { it.stage == "rem" })
        assertTrue(snapshot.hasData)
    }

    @Test
    fun aWorkoutCrossingMidnightStaysOnTheDayItStarted() {
        val day = LocalDate.of(2026, 11, 15)
        val start = day.atTime(23, 30).atZone(zone).toInstant()
        val end = day.plusDays(1).atTime(0, 30).atZone(zone).toInstant()
        val records = HealthRecords(
            exercise = listOf(
                ExerciseSample(
                    id = "workout-1",
                    start = start,
                    end = end,
                    discipline = "run",
                    deviceKind = "watch",
                    watchVerified = true,
                    distanceMeters = 1000.0,
                ),
            ),
        )

        val first = HealthDayAggregator.aggregate(records, day, zone)
        val second = HealthDayAggregator.aggregate(records, day.plusDays(1), zone)

        assertEquals(1, first.workouts.size)
        assertEquals("workout-1", first.workouts.first().externalId)
        assertEquals(Duration.ofHours(1).toMillis(), first.workouts.first().durationMs)
        assertTrue(second.workouts.isEmpty())
    }

    @Test
    fun daylightSavingDaysUseTheConfiguredZoneBoundaries() {
        val longDay = LocalDate.of(2026, 10, 25)
        val shortDay = LocalDate.of(2026, 3, 29)

        val longSnapshot = HealthDayAggregator.aggregate(
            HealthRecords(
                sleep = listOf(
                    SleepSample(
                        id = "long-sleep",
                        start = longDay.atStartOfDay(zone).toInstant(),
                        end = longDay.plusDays(1).atStartOfDay(zone).toInstant(),
                    ),
                ),
            ),
            longDay,
            zone,
        )
        val shortSnapshot = HealthDayAggregator.aggregate(
            HealthRecords(
                sleep = listOf(
                    SleepSample(
                        id = "short-sleep",
                        start = shortDay.atStartOfDay(zone).toInstant(),
                        end = shortDay.plusDays(1).atStartOfDay(zone).toInstant(),
                    ),
                ),
            ),
            shortDay,
            zone,
        )

        assertEquals(90_000_000L, longSnapshot.sleepMs)
        assertEquals(82_800_000L, shortSnapshot.sleepMs)
    }

    @Test
    fun duplicateSourceRecordsAreCountedOnce() {
        val day = LocalDate.of(2026, 11, 16)
        val time = day.atTime(12, 0).atZone(zone).toInstant()
        val records = HealthRecords(
            steps = listOf(
                StepsSample("steps-1", time, time.plusSeconds(60), 5),
                StepsSample("steps-1", time, time.plusSeconds(60), 5),
                StepsSample("steps-2", time, time.plusSeconds(60), 7),
            ),
            hrvRmssd = listOf(
                HrvRmssdSample("hrv-1", time, 42.0),
                HrvRmssdSample("hrv-1", time, 42.0),
            ),
        )

        val snapshot = HealthDayAggregator.aggregate(records, day, zone)

        assertEquals(12L, snapshot.steps)
        assertEquals(42.0, snapshot.hrvRmssdMs!!, 0.0)
    }

    @Test
    fun repeatedReadsDoNotAccumulate() {
        val day = LocalDate.of(2026, 11, 16)
        val time = day.atTime(9, 0).atZone(zone).toInstant()
        val records = HealthRecords(
            steps = listOf(StepsSample("steps-1", time, time.plusSeconds(60), 100)),
        )

        val first = HealthDayAggregator.aggregate(records, day, zone)
        val second = HealthDayAggregator.aggregate(records, day, zone)

        assertEquals(first, second)
        assertEquals(100L, second.steps)
    }

    @Test
    fun zeroStepsAndAbsentStepsAreDifferent() {
        val day = LocalDate.of(2026, 11, 16)
        val time = day.atTime(9, 0).atZone(zone).toInstant()

        val zero = HealthDayAggregator.aggregate(
            HealthRecords(steps = listOf(StepsSample("zero", time, time.plusSeconds(60), 0))),
            day,
            zone,
        )
        val absent = HealthDayAggregator.aggregate(HealthRecords(), day, zone)

        assertEquals(0L, zero.steps)
        assertNull(absent.steps)
    }

    @Test
    fun theFakeReaderIsTheUnitTestSeam() = runTest {
        val day = LocalDate.of(2026, 11, 16)
        val time = day.atTime(8, 0).atZone(zone).toInstant()
        val reader = FakeHealthReader(
            records = HealthRecords(
                restingHeartRate = listOf(dev.temper.health.RestingHeartRateSample("rhr-1", time, 52)),
            ),
        )

        val records = reader.read(day, zone, HealthPermission.all.toSet())
        val snapshot = HealthDayAggregator.aggregate(records, day, zone)

        assertEquals(1, reader.reads)
        assertEquals(52L, snapshot.restingHrBpm)
        assertNull(snapshot.steps)
    }

    @Test
    fun theUploadKeyIsStableForADayAndRevision() {
        assertEquals("day-2026-11-16-0", healthDayKey(LocalDate.of(2026, 11, 16), 0))
        assertEquals("day-2026-11-16-3", healthDayKey(LocalDate.of(2026, 11, 16), 3))
    }

    @Test
    fun absentMeasurementsAreOmittedAndZeroIsNot() {
        val request = DayUploadRequest(
            date = "2026-11-16",
            timezone = "Europe/Madrid",
            expectedRevision = 0,
            source = "health_connect",
            metrics = DayMetrics(sleepMs = 25_200_000, steps = 0),
            sleepStages = listOf(HealthSleepStage("deep", 7_200_000)),
            workouts = emptyList(),
        )

        val body = TemperJson.encodeToString(request)

        assertTrue(body.contains("\"source\":\"health_connect\""))
        assertTrue(body.contains("\"expected_revision\":0"))
        assertTrue(body.contains("\"sleep_ms\":25200000"))
        assertTrue(body.contains("\"steps\":0"))
        assertFalse(body.contains("resting_hr_bpm"))
        assertFalse(body.contains("hrv_rmssd_ms"))
        assertTrue(body.contains("\"sleep_stages\":[{\"stage\":\"deep\",\"duration_ms\":7200000}]"))
        assertTrue(body.contains("\"workouts\":[]"))
    }

    @Test
    fun aWorkoutCarriesItsOriginAndWatchFlag() {
        val workout = DayWorkout(
            origin = "health_connect",
            externalId = "record-1",
            deviceKind = "watch",
            startedAt = 1_763_000_000_000,
            endedAt = 1_763_003_600_000,
            discipline = "run",
            durationMs = 3_600_000,
            distanceM = 5200.0,
            watchVerified = false,
        )

        val body = TemperJson.encodeToString(workout)

        assertTrue(body.contains("\"origin\":\"health_connect\""))
        assertTrue(body.contains("\"watch_verified\":false"))
        assertTrue(body.contains("\"external_id\":\"record-1\""))
    }

    @Test
    fun aDayResponseKeepsAnUnavailableHrvAbsent() {
        val body = """
            {
              "day": {
                "date": "2026-11-16",
                "timezone": "Europe/Madrid",
                "revision": 2,
                "sleep_ms": 25200000,
                "steps": 0,
                "resting_hr_bpm": 52,
                "sleep_stages": [{"stage": "deep", "duration_ms": 7200000}],
                "workouts": [],
                "server_field": "ignored"
              }
            }
        """.trimIndent()

        val day = TemperJson.decodeFromString<dev.temper.model.HealthDayEnvelope>(body).day

        assertEquals(2L, day.revision)
        assertEquals(0L, day.steps)
        assertNull(day.hrvRmssdMs)
        assertEquals(1, day.sleepStages.size)
    }
}
