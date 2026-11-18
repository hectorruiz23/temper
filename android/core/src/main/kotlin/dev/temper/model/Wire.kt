package dev.temper.model

import dev.temper.health.HealthWorkout
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class HealthResponse(
    val ok: Boolean = false,
)

@Serializable
data class WeekResponse(
    @SerialName("week_start") val weekStart: String = "",
    @SerialName("week_end") val weekEnd: String = "",
    val planned: List<PlannedSession> = emptyList(),
    val performed: List<PerformedSession> = emptyList(),
    val days: List<WeekDay> = emptyList(),
)

@Serializable
data class PlannedSession(
    val date: String = "",
    @SerialName("template_id") val templateId: String = "",
    val discipline: String = "",
    val target: String = "",
    @SerialName("plan_version_id") val planVersionId: Long = 0,
)

@Serializable
data class PerformedSession(
    val id: String? = null,
    val date: String? = null,
    @SerialName("template_id") val templateId: String? = null,
    val discipline: String? = null,
    @SerialName("started_at") val startedAt: String? = null,
    val revision: Long = 1,
    val complete: Boolean = false,
    val evidence: Evidence? = null,
)

@Serializable
data class WeekDay(
    val date: String = "",
    val revision: Long = 0,
    @SerialName("sleep_ms") val sleepMs: Long? = null,
    val steps: Long? = null,
    @SerialName("resting_hr_bpm") val restingHrBpm: Long? = null,
    @SerialName("hrv_rmssd_ms") val hrvRmssdMs: Double? = null,
)

@Serializable
data class PlanEnvelope(
    val plan: Plan,
)

@Serializable
data class Plan(
    val id: Long = 0,
    @SerialName("created_at") val createdAt: String = "",
    val reason: String = "",
    @SerialName("effective_from") val effectiveFrom: String = "",
    val timezone: String = "",
    @SerialName("weekly_frequency") val weeklyFrequency: Int = 0,
    @SerialName("goal_id") val goalId: String = "",
    val sessions: List<PlanSession> = emptyList(),
)

@Serializable
data class PlanSession(
    val id: String = "",
    val position: Int = 0,
    @SerialName("scheduled_date") val scheduledDate: String? = null,
    val weekday: Int? = null,
    val discipline: String = "",
    val target: String = "",
    val exercises: List<PlanExercise> = emptyList(),
    @SerialName("interval_steps") val intervalSteps: List<IntervalStep> = emptyList(),
)

@Serializable
data class PlanExercise(
    val position: Int = 0,
    @SerialName("exercise_id") val exerciseId: String = "",
    @SerialName("target_sets") val targetSets: Int? = null,
    @SerialName("reps_min") val repsMin: Int? = null,
    @SerialName("reps_max") val repsMax: Int? = null,
    @SerialName("target_kg") val targetKg: Double? = null,
    val tempo: String? = null,
)

@Serializable
data class IntervalStep(
    val position: Int = 0,
    val label: String = "",
    @SerialName("duration_ms") val durationMs: Int? = null,
    @SerialName("distance_m") val distanceM: Double? = null,
)

@Serializable
data class ExerciseListResponse(
    val exercises: List<Exercise> = emptyList(),
    @SerialName("next_cursor") val nextCursor: String? = null,
)

@Serializable
data class Exercise(
    val id: String = "",
    val name: String = "",
    @SerialName("movement_pattern") val movementPattern: String = "",
    val unilateral: Boolean = false,
    @SerialName("created_at") val createdAt: Long = 0,
    val equipment: List<ExerciseEquipment> = emptyList(),
)

@Serializable
data class ExerciseEquipment(
    @SerialName("equipment_id") val equipmentId: String = "",
    val quantity: Int = 0,
)

@Serializable
data class SessionEnvelope(
    val session: Session,
)

@Serializable
data class Session(
    val id: String = "",
    @SerialName("template_id") val templateId: String? = null,
    val date: String = "",
    val discipline: String = "",
    @SerialName("started_at") val startedAt: String? = null,
    @SerialName("reported_completed_at") val reportedCompletedAt: String? = null,
    @SerialName("watch_workout_id") val watchWorkoutId: String? = null,
    @SerialName("created_at") val createdAt: String = "",
    val revision: Long = 0,
    val complete: Boolean = false,
)

@Serializable
data class CreateSessionRequest(
    val id: String,
    @SerialName("template_id") val templateId: String? = null,
    val date: String,
    val discipline: String,
    @SerialName("started_at") val startedAt: String? = null,
)

@Serializable
data class LogSetsRequest(
    @SerialName("session_id") val sessionId: String,
    val sets: List<LogSet>,
)

@Serializable
data class LogSet(
    @SerialName("exercise_id") val exerciseId: String,
    @SerialName("set_index") val setIndex: Int,
    val kg: Double,
    val reps: Int,
    val note: String? = null,
)

@Serializable
data class SetsEnvelope(
    val sets: List<LoggedSet> = emptyList(),
    val echo: BatchEcho = BatchEcho(),
)

@Serializable
data class LoggedSet(
    val id: String = "",
    @SerialName("session_id") val sessionId: String = "",
    @SerialName("exercise_id") val exerciseId: String = "",
    @SerialName("set_index") val setIndex: Int = 0,
    val kg: Double = 0.0,
    val reps: Int = 0,
    val note: String? = null,
    @SerialName("evidence_source") val evidenceSource: String = "",
    @SerialName("created_at") val createdAt: String = "",
    val revision: Long = 0,
)

@Serializable
data class BatchEcho(
    @SerialName("session_id") val sessionId: String = "",
    val sets: List<EchoSet> = emptyList(),
)

@Serializable
data class EchoSet(
    @SerialName("exercise_id") val exerciseId: String = "",
    @SerialName("set_index") val setIndex: Int = 0,
    val kg: Double = 0.0,
    val reps: Int = 0,
    val note: String? = null,
)

@Serializable
data class CheckInRequest(
    @SerialName("expected_revision") val expectedRevision: Long,
    @SerialName("reported_completed_at") val reportedCompletedAt: String? = null,
    @SerialName("watch_workout_id") val watchWorkoutId: String? = null,
)

@Serializable
data class CheckInEnvelope(
    val session: Session,
    @SerialName("effective_evidence") val effectiveEvidence: Evidence? = null,
)

@Serializable
data class Evidence(
    val source: String = "",
    @SerialName("workout_id") val workoutId: String? = null,
    @SerialName("reported_completed_at") val reportedCompletedAt: String? = null,
)

@Serializable
data class ApiErrorEnvelope(
    val error: ApiErrorDetail,
)

@Serializable
data class ApiErrorDetail(
    val code: String = "",
    val message: String = "",
)

@Serializable
data class HealthDayEnvelope(
    val day: HealthDay,
)

@Serializable
data class HealthDay(
    val date: String = "",
    val timezone: String = "",
    val revision: Long = 0,
    @SerialName("sleep_ms") val sleepMs: Long? = null,
    val steps: Long? = null,
    @SerialName("resting_hr_bpm") val restingHrBpm: Long? = null,
    @SerialName("hrv_rmssd_ms") val hrvRmssdMs: Double? = null,
    @SerialName("updated_at") val updatedAt: Long = 0,
    @SerialName("last_source") val lastSource: String = "",
    @SerialName("sleep_stages") val sleepStages: List<HealthSleepStage> = emptyList(),
    val workouts: List<HealthWorkoutWire> = emptyList(),
)

@Serializable
data class HealthSleepStage(
    val stage: String = "",
    @SerialName("duration_ms") val durationMs: Long = 0,
)

@Serializable
data class HealthWorkoutWire(
    val id: String = "",
    val origin: String = "",
    @SerialName("external_id") val externalId: String = "",
    @SerialName("device_kind") val deviceKind: String? = null,
    @SerialName("started_at") val startedAt: Long = 0,
    @SerialName("ended_at") val endedAt: Long = 0,
    val discipline: String = "",
    @SerialName("duration_ms") val durationMs: Long = 0,
    @SerialName("distance_m") val distanceM: Double? = null,
    @SerialName("avg_hr_bpm") val avgHrBpm: Long? = null,
    @SerialName("max_hr_bpm") val maxHrBpm: Long? = null,
    @SerialName("calories_kcal") val caloriesKcal: Double? = null,
    @SerialName("pace_sec_per_km") val paceSecPerKm: Double? = null,
    @SerialName("splits") val splits: List<DayWorkoutSplit>? = null,
    @SerialName("watch_verified") val watchVerified: Boolean = false,
)

@Serializable
data class DayUploadRequest(
    val date: String,
    val timezone: String,
    @SerialName("expected_revision") val expectedRevision: Long,
    val source: String,
    val metrics: DayMetrics? = null,
    @SerialName("sleep_stages") val sleepStages: List<HealthSleepStage>? = null,
    val workouts: List<DayWorkout>? = null,
)

@Serializable
data class DayMetrics(
    @SerialName("sleep_ms") val sleepMs: Long? = null,
    val steps: Long? = null,
    @SerialName("resting_hr_bpm") val restingHrBpm: Long? = null,
    @SerialName("hrv_rmssd_ms") val hrvRmssdMs: Double? = null,
) {
    val isEmpty: Boolean
        get() = sleepMs == null && steps == null && restingHrBpm == null && hrvRmssdMs == null
}

@Serializable
data class DayWorkout(
    val origin: String,
    @SerialName("external_id") val externalId: String,
    @SerialName("device_kind") val deviceKind: String? = null,
    @SerialName("started_at") val startedAt: Long,
    @SerialName("ended_at") val endedAt: Long,
    val discipline: String,
    @SerialName("duration_ms") val durationMs: Long,
    @SerialName("distance_m") val distanceM: Double? = null,
    @SerialName("avg_hr_bpm") val avgHrBpm: Long? = null,
    @SerialName("max_hr_bpm") val maxHrBpm: Long? = null,
    @SerialName("calories_kcal") val caloriesKcal: Double? = null,
    @SerialName("pace_sec_per_km") val paceSecPerKm: Double? = null,
    val splits: List<DayWorkoutSplit>? = null,
    @SerialName("watch_verified") val watchVerified: Boolean,
)

@Serializable
data class DayWorkoutSplit(
    val position: Int,
    val kind: String? = null,
    @SerialName("distance_m") val distanceM: Double? = null,
    @SerialName("duration_ms") val durationMs: Long? = null,
    @SerialName("avg_hr_bpm") val avgHrBpm: Long? = null,
    @SerialName("pace_sec_per_km") val paceSecPerKm: Double? = null,
)

fun HealthWorkout.toDayWorkout(): DayWorkout = DayWorkout(
    origin = "health_connect",
    externalId = externalId,
    deviceKind = deviceKind,
    startedAt = startedAt.toEpochMilli(),
    endedAt = endedAt.toEpochMilli(),
    discipline = discipline,
    durationMs = durationMs,
    distanceM = distanceMeters,
    avgHrBpm = avgHeartRateBpm,
    maxHrBpm = maxHeartRateBpm,
    caloriesKcal = caloriesKcal,
    paceSecPerKm = paceSecPerKm,
    splits = splits.map { split ->
        DayWorkoutSplit(
            position = split.position,
            kind = split.kind,
            distanceM = split.distanceMeters,
            durationMs = split.durationMs,
            avgHrBpm = split.avgHeartRateBpm,
            paceSecPerKm = split.paceSecPerKm,
        )
    }.takeIf { it.isNotEmpty() },
    watchVerified = watchVerified,
)
