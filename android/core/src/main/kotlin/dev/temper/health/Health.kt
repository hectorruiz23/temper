package dev.temper.health

import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.TextStyle
import java.util.Locale

enum class HealthAvailability {
    Available,
    NotInstalled,
    UpdateRequired,
}

enum class HealthPermission(val permission: String, val label: String) {
    Sleep("android.permission.health.READ_SLEEP", "Sleep"),
    Steps("android.permission.health.READ_STEPS", "Steps"),
    RestingHeartRate("android.permission.health.READ_RESTING_HEART_RATE", "Resting HR"),
    HeartRateVariability("android.permission.health.READ_HEART_RATE_VARIABILITY", "HRV"),
    Exercise("android.permission.health.READ_EXERCISE", "Workouts");

    companion object {
        val all: List<HealthPermission> = entries
        fun fromPermission(value: String): HealthPermission? = entries.firstOrNull { it.permission == value }
    }
}

enum class HealthPermissionState {
    Unknown,
    Granted,
    Partial,
    Denied,
    Revoked,
}

data class SleepStageSample(
    val stage: String,
    val start: Instant,
    val end: Instant,
)

data class SleepSample(
    val id: String,
    val start: Instant,
    val end: Instant,
    val stages: List<SleepStageSample> = emptyList(),
)

data class StepsSample(
    val id: String,
    val start: Instant,
    val end: Instant,
    val count: Long,
)

data class RestingHeartRateSample(
    val id: String,
    val time: Instant,
    val beatsPerMinute: Long,
)

data class HrvRmssdSample(
    val id: String,
    val time: Instant,
    val rmssdMs: Double,
)

data class ExerciseSample(
    val id: String,
    val start: Instant,
    val end: Instant,
    val discipline: String,
    val deviceKind: String? = null,
    val watchVerified: Boolean = false,
    val distanceMeters: Double? = null,
)

data class HealthRecords(
    val sleep: List<SleepSample> = emptyList(),
    val steps: List<StepsSample> = emptyList(),
    val restingHeartRate: List<RestingHeartRateSample> = emptyList(),
    val hrvRmssd: List<HrvRmssdSample> = emptyList(),
    val exercise: List<ExerciseSample> = emptyList(),
)

data class SleepStageTotal(
    val stage: String,
    val durationMs: Long,
)

data class HealthWorkout(
    val externalId: String,
    val deviceKind: String?,
    val startedAt: Instant,
    val endedAt: Instant,
    val discipline: String,
    val durationMs: Long,
    val distanceMeters: Double?,
    val watchVerified: Boolean,
)

data class HealthDaySnapshot(
    val date: LocalDate,
    val timezone: ZoneId,
    val sleepMs: Long? = null,
    val steps: Long? = null,
    val restingHrBpm: Long? = null,
    val hrvRmssdMs: Double? = null,
    val sleepStages: List<SleepStageTotal> = emptyList(),
    val workouts: List<HealthWorkout> = emptyList(),
) {
    val hasData: Boolean
        get() = sleepMs != null ||
            steps != null ||
            restingHrBpm != null ||
            hrvRmssdMs != null ||
            sleepStages.isNotEmpty() ||
            workouts.isNotEmpty()
}

interface HealthReader {
    suspend fun availability(): HealthAvailability

    suspend fun grantedPermissions(): Set<HealthPermission>

    suspend fun read(day: LocalDate, zone: ZoneId, permissions: Set<HealthPermission>): HealthRecords
}

fun healthDayKey(date: LocalDate, revision: Long): String = "day-$date-$revision"

fun lastSevenDays(today: LocalDate): List<LocalDate> = List(7) { today.minusDays(it.toLong()) }

fun healthDaysLabel(days: List<LocalDate>): String =
    days.sorted()
        .groupBy { it.month }
        .entries
        .joinToString(", ") { (month, dates) ->
            dates.joinToString(", ") { it.dayOfMonth.toString() } + " " +
                month.getDisplayName(TextStyle.SHORT, Locale.ENGLISH)
        }
