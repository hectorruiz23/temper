package dev.temper.health

import java.time.Duration
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

object HealthDayAggregator {
    private val stageOrder = listOf(
        "deep",
        "light",
        "rem",
        "awake",
        "sleeping",
        "out_of_bed",
        "awake_in_bed",
        "unknown",
    )

    fun aggregate(records: HealthRecords, day: LocalDate, zone: ZoneId): HealthDaySnapshot {
        val start = day.atStartOfDay(zone).toInstant()
        val end = day.plusDays(1).atStartOfDay(zone).toInstant()

        val sleepSessions = records.sleep.uniqueBy { it.id }.filter { overlaps(it.start, it.end, start, end) }
        val sleepMs = if (sleepSessions.isEmpty()) {
            null
        } else {
            sleepSessions.sumOf { overlapMillis(it.start, it.end, start, end) }
        }

        val stages = sleepSessions
            .flatMap { it.stages }
            .filter { overlaps(it.start, it.end, start, end) }
            .groupBy { normalizeStage(it.stage) }
            .map { (stage, samples) ->
                SleepStageTotal(stage = stage, durationMs = samples.sumOf { overlapMillis(it.start, it.end, start, end) })
            }
            .sortedBy { stageRank(it.stage) }

        val stepsSamples = records.steps.uniqueBy { it.id }.filter { within(it.start, start, end) }
        val steps = if (stepsSamples.isEmpty()) null else stepsSamples.sumOf { it.count }

        val restingHr = records.restingHeartRate
            .uniqueBy { it.id }
            .filter { within(it.time, start, end) && it.beatsPerMinute > 0 }
            .maxByOrNull { it.time }
            ?.beatsPerMinute

        val hrv = records.hrvRmssd
            .uniqueBy { it.id }
            .filter { within(it.time, start, end) && it.rmssdMs >= 0.0 }
            .maxByOrNull { it.time }
            ?.rmssdMs

        val workouts = records.exercise
            .uniqueBy { it.id }
            .filter { it.id.isNotBlank() && within(it.start, start, end) }
            .map { sample -> WorkoutMetrics.workout(sample, records) }

        return HealthDaySnapshot(
            date = day,
            timezone = zone,
            sleepMs = sleepMs,
            steps = steps,
            restingHrBpm = restingHr,
            hrvRmssdMs = hrv,
            sleepStages = stages,
            workouts = workouts,
        )
    }

    private fun overlaps(start: Instant, end: Instant, dayStart: Instant, dayEnd: Instant): Boolean =
        end.isAfter(dayStart) && start.isBefore(dayEnd)

    private fun within(value: Instant, dayStart: Instant, dayEnd: Instant): Boolean =
        !value.isBefore(dayStart) && value.isBefore(dayEnd)

    private fun overlapMillis(start: Instant, end: Instant, dayStart: Instant, dayEnd: Instant): Long {
        val clippedStart = if (start.isBefore(dayStart)) dayStart else start
        val clippedEnd = if (end.isAfter(dayEnd)) dayEnd else end
        return Duration.between(clippedStart, clippedEnd).toMillis().coerceAtLeast(0)
    }

    private fun normalizeStage(value: String): String {
        val stage = value.trim().lowercase()
        return if (stage.isEmpty()) "unknown" else stage
    }

    private fun stageRank(value: String): Int = stageOrder.indexOf(value).let { if (it < 0) stageOrder.size else it }

    private fun <T> List<T>.uniqueBy(id: (T) -> String): List<T> {
        val seen = HashSet<String>()
        return filter { item ->
            val key = id(item)
            key.isBlank() || seen.add(key)
        }
    }
}
