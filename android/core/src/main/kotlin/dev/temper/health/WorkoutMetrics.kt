package dev.temper.health

import java.time.Duration
import java.time.Instant
import kotlin.math.round
import kotlin.math.roundToLong

object WorkoutMetrics {
    private val originOrder = listOf(
        "watch",
        "fitness_band",
        "ring",
        "chest_strap",
        "phone",
        "smart_display",
        "scale",
        "head_mounted",
        "other",
    )

    fun workout(sample: ExerciseSample, records: HealthRecords): HealthWorkout {
        val endedAt = if (sample.end.isBefore(sample.start)) sample.start else sample.end
        val durationMs = Duration.between(sample.start, endedAt).toMillis().coerceAtLeast(0)

        val distanceRecords = chosenOrigin(
            items = records.distance.distinctBy { it.id },
            start = sample.start,
            end = endedAt,
            origin = { it.origin },
            from = { it.start },
            until = { it.end },
        )
        val recordMeters = distanceRecords.sumOf { it.meters }.takeIf { it > 0.0 }
        val laps = sample.laps.filter { it.meters != null && it.meters > 0.0 }
        val lapMeters = laps.sumOf { it.meters ?: 0.0 }.takeIf { it > 0.0 }
        val distanceMeters = recordMeters ?: lapMeters
        val distanceSource = when {
            recordMeters != null -> WorkoutDistanceSource.Records
            lapMeters != null -> WorkoutDistanceSource.Laps
            else -> WorkoutDistanceSource.Absent
        }

        val calories = chosenOrigin(
            items = records.calories.distinctBy { it.id },
            start = sample.start,
            end = endedAt,
            origin = { it.origin },
            from = { it.start },
            until = { it.end },
        )
        val caloriesKcal = calories.sumOf { it.kilocalories }.takeIf { it > 0.0 }

        val heartRate = records.heartRate
            .distinctBy { it.id }
            .filter { it.beatsPerMinute > 0 && !it.time.isBefore(sample.start) && !it.time.isAfter(endedAt) }
        val heartRateOrigin = heartRate.groupBy { normalizeOrigin(it.origin) }.keys.minByOrNull { originRank(it) }
        val beats = heartRate.filter { normalizeOrigin(it.origin) == heartRateOrigin }.map { it.beatsPerMinute }
        val averageHeartRate = beats.takeIf { it.isNotEmpty() }
            ?.average()
            ?.roundToLong()
            ?.takeIf { it > 0 }

        return HealthWorkout(
            externalId = sample.id,
            deviceKind = sample.deviceKind,
            startedAt = sample.start,
            endedAt = endedAt,
            discipline = sample.discipline.ifBlank { "other" },
            durationMs = durationMs,
            distanceMeters = distanceMeters,
            watchVerified = sample.watchVerified,
            distanceSource = distanceSource,
            paceSecPerKm = pace(distanceMeters, durationMs),
            avgHeartRateBpm = averageHeartRate,
            maxHeartRateBpm = beats.maxOrNull(),
            caloriesKcal = caloriesKcal,
            splits = splits(laps, distanceRecords),
        )
    }

    private fun splits(
        laps: List<ExerciseLapSample>,
        distanceRecords: List<DistanceSample>,
    ): List<WorkoutSplitSample> = when {
        laps.isNotEmpty() -> laps.mapIndexed { index, lap ->
            val durationMs = Duration.between(lap.start, lap.end).toMillis().coerceAtLeast(0)
            WorkoutSplitSample(
                position = index,
                kind = "lap",
                distanceMeters = lap.meters,
                durationMs = durationMs,
                paceSecPerKm = pace(lap.meters, durationMs),
            )
        }

        distanceRecords.size > 1 -> distanceRecords.sortedBy { it.start }.mapIndexed { index, record ->
            val durationMs = Duration.between(record.start, record.end).toMillis().coerceAtLeast(0)
            WorkoutSplitSample(
                position = index,
                kind = "segment",
                distanceMeters = record.meters,
                durationMs = durationMs,
                paceSecPerKm = pace(record.meters, durationMs),
            )
        }

        else -> emptyList()
    }

    private fun pace(distanceMeters: Double?, durationMs: Long): Double? {
        if (distanceMeters == null || distanceMeters <= 0.0 || durationMs <= 0) return null
        val secondsPerKilometre = durationMs.toDouble() / distanceMeters
        return round(secondsPerKilometre * 10.0) / 10.0
    }

    private fun <T> chosenOrigin(
        items: List<T>,
        start: Instant,
        end: Instant,
        origin: (T) -> String,
        from: (T) -> Instant,
        until: (T) -> Instant,
    ): List<T> {
        val grouped = items
            .filter { until(it).isAfter(start) && from(it).isBefore(end) }
            .groupBy { normalizeOrigin(origin(it)) }
        val chosen = grouped.keys.minByOrNull { originRank(it) } ?: return emptyList()
        return grouped.getValue(chosen)
    }

    private fun normalizeOrigin(value: String): String {
        val origin = value.trim().lowercase()
        return if (origin.isEmpty()) "other" else origin
    }

    private fun originRank(value: String): Int = originOrder.indexOf(value).let { if (it < 0) originOrder.size else it }
}
