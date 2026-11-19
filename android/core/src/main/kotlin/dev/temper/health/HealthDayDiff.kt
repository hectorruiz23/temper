package dev.temper.health

import dev.temper.model.DayWorkoutSplit
import dev.temper.model.HealthDay
import dev.temper.model.HealthWorkoutWire

object HealthDayDiff {
    fun sameDay(existing: HealthDay, snapshot: HealthDaySnapshot, granted: Set<HealthPermission>): Boolean {
        if (existing.timezone != snapshot.timezone.id) return false
        if (HealthPermission.Sleep in granted) {
            if (snapshot.sleepMs != null && existing.sleepMs != snapshot.sleepMs) return false
            if (existing.sleepStages.map { it.stage to it.durationMs }.sortedBy { it.first } !=
                snapshot.sleepStages.map { it.stage to it.durationMs }.sortedBy { it.first }
            ) {
                return false
            }
        }
        if (HealthPermission.Steps in granted) {
            if (snapshot.steps != null && existing.steps != snapshot.steps) return false
            if (snapshot.stepsSource != null && existing.stepsSource != snapshot.stepsSource) return false
        }
        if (HealthPermission.RestingHeartRate in granted &&
            snapshot.restingHrBpm != null &&
            existing.restingHrBpm != snapshot.restingHrBpm
        ) {
            return false
        }
        if (HealthPermission.HeartRateVariability in granted &&
            snapshot.hrvRmssdMs != null &&
            existing.hrvRmssdMs != snapshot.hrvRmssdMs
        ) {
            return false
        }
        if (HealthPermission.Exercise in granted) {
            if (existing.workouts.map { it.signature() }.sorted() != snapshot.workouts.map { it.signature() }.sorted()) {
                return false
            }
        }
        return true
    }

    private fun HealthWorkoutWire.signature(): String = listOf(
        externalId,
        deviceKind.orEmpty(),
        startedAt.toString(),
        endedAt.toString(),
        discipline,
        durationMs.toString(),
        distanceM?.toString().orEmpty(),
        avgHrBpm?.toString().orEmpty(),
        maxHrBpm?.toString().orEmpty(),
        caloriesKcal?.toString().orEmpty(),
        paceSecPerKm?.toString().orEmpty(),
        splits.orEmpty().map { it.signature() }.joinToString(","),
        watchVerified.toString(),
    ).joinToString("|")

    private fun HealthWorkout.signature(): String = listOf(
        externalId,
        deviceKind.orEmpty(),
        startedAt.toEpochMilli().toString(),
        endedAt.toEpochMilli().toString(),
        discipline,
        durationMs.toString(),
        distanceMeters?.toString().orEmpty(),
        avgHeartRateBpm?.toString().orEmpty(),
        maxHeartRateBpm?.toString().orEmpty(),
        caloriesKcal?.toString().orEmpty(),
        paceSecPerKm?.toString().orEmpty(),
        splits.map { it.signature() }.joinToString(","),
        watchVerified.toString(),
    ).joinToString("|")

    private fun DayWorkoutSplit.signature(): String = listOf(
        position.toString(),
        kind.orEmpty(),
        distanceM?.toString().orEmpty(),
        durationMs?.toString().orEmpty(),
        avgHrBpm?.toString().orEmpty(),
        paceSecPerKm?.toString().orEmpty(),
    ).joinToString(":")

    private fun WorkoutSplitSample.signature(): String = listOf(
        position.toString(),
        kind.orEmpty(),
        distanceMeters?.toString().orEmpty(),
        durationMs?.toString().orEmpty(),
        avgHeartRateBpm?.toString().orEmpty(),
        paceSecPerKm?.toString().orEmpty(),
    ).joinToString(":")
}
