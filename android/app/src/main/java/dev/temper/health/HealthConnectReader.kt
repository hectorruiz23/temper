package dev.temper.health

import android.content.Context
import androidx.health.connect.client.HealthConnectClient
import androidx.health.connect.client.records.DistanceRecord
import androidx.health.connect.client.records.ExerciseSessionRecord as HealthExerciseSession
import androidx.health.connect.client.records.HeartRateRecord
import androidx.health.connect.client.records.HeartRateVariabilityRmssdRecord
import androidx.health.connect.client.records.Record
import androidx.health.connect.client.records.RestingHeartRateRecord
import androidx.health.connect.client.records.SleepSessionRecord as HealthSleepSession
import androidx.health.connect.client.records.StepsRecord
import androidx.health.connect.client.records.TotalCaloriesBurnedRecord
import androidx.health.connect.client.records.metadata.Device
import androidx.health.connect.client.records.metadata.Metadata
import androidx.health.connect.client.request.ReadRecordsRequest
import androidx.health.connect.client.time.TimeRangeFilter
import java.time.LocalDate
import java.time.ZoneId
import kotlin.reflect.KClass

class HealthConnectReader(context: Context) : HealthReader {
    private val appContext = context.applicationContext

    override suspend fun availability(): HealthAvailability = when (HealthConnectClient.getSdkStatus(appContext)) {
        HealthConnectClient.SDK_AVAILABLE -> HealthAvailability.Available
        HealthConnectClient.SDK_UNAVAILABLE_PROVIDER_UPDATE_REQUIRED -> HealthAvailability.UpdateRequired
        else -> HealthAvailability.NotInstalled
    }

    override suspend fun grantedPermissions(): Set<HealthPermission> {
        val client = HealthConnectClient.getOrCreate(appContext)
        val granted = client.permissionController.getGrantedPermissions()
        return HealthPermission.all.filterTo(mutableSetOf()) { it.permission in granted }
    }

    override suspend fun read(
        day: LocalDate,
        zone: ZoneId,
        permissions: Set<HealthPermission>,
    ): HealthRecords {
        val client = HealthConnectClient.getOrCreate(appContext)
        val start = day.atStartOfDay(zone).toInstant()
        val end = day.plusDays(1).atStartOfDay(zone).toInstant()
        val range = TimeRangeFilter.between(start, end)

        return HealthRecords(
            sleep = if (HealthPermission.Sleep in permissions) {
                readAll(client, HealthSleepSession::class, range).map { record ->
                    SleepSample(
                        id = stableId(record.metadata),
                        start = record.startTime,
                        end = record.endTime,
                        stages = record.stages.map { stage ->
                            SleepStageSample(
                                stage = sleepStageName(stage.stage),
                                start = stage.startTime,
                                end = stage.endTime,
                            )
                        },
                    )
                }
            } else {
                emptyList()
            },
            steps = if (HealthPermission.Steps in permissions) {
                readAll(client, StepsRecord::class, range).map { record ->
                    StepsSample(
                        id = stableId(record.metadata),
                        start = record.startTime,
                        end = record.endTime,
                        count = record.count,
                    )
                }
            } else {
                emptyList()
            },
            restingHeartRate = if (HealthPermission.RestingHeartRate in permissions) {
                readAll(client, RestingHeartRateRecord::class, range).map { record ->
                    RestingHeartRateSample(
                        id = stableId(record.metadata),
                        time = record.time,
                        beatsPerMinute = record.beatsPerMinute,
                    )
                }
            } else {
                emptyList()
            },
            hrvRmssd = if (HealthPermission.HeartRateVariability in permissions) {
                readAll(client, HeartRateVariabilityRmssdRecord::class, range).map { record ->
                    HrvRmssdSample(
                        id = stableId(record.metadata),
                        time = record.time,
                        rmssdMs = record.heartRateVariabilityMillis,
                    )
                }
            } else {
                emptyList()
            },
            exercise = if (HealthPermission.Exercise in permissions) {
                readAll(client, HealthExerciseSession::class, range).map { record ->
                    val device = record.metadata.device
                    ExerciseSample(
                        id = stableId(record.metadata),
                        start = record.startTime,
                        end = record.endTime,
                        discipline = discipline(record.exerciseType),
                        deviceKind = deviceKind(device?.type),
                        watchVerified = device?.type == Device.TYPE_WATCH,
                        laps = record.laps.map { lap ->
                            ExerciseLapSample(
                                start = lap.startTime,
                                end = lap.endTime,
                                meters = lap.length?.inMeters,
                            )
                        },
                    )
                }
            } else {
                emptyList()
            },
            distance = if (HealthPermission.Distance in permissions) {
                readAll(client, DistanceRecord::class, range).map { record ->
                    DistanceSample(
                        id = stableId(record.metadata),
                        start = record.startTime,
                        end = record.endTime,
                        meters = record.distance.inMeters,
                        origin = deviceKind(record.metadata.device?.type) ?: "other",
                    )
                }
            } else {
                emptyList()
            },
            calories = if (HealthPermission.TotalCaloriesBurned in permissions) {
                readAll(client, TotalCaloriesBurnedRecord::class, range).map { record ->
                    CaloriesSample(
                        id = stableId(record.metadata),
                        start = record.startTime,
                        end = record.endTime,
                        kilocalories = record.energy.inKilocalories,
                        origin = deviceKind(record.metadata.device?.type) ?: "other",
                    )
                }
            } else {
                emptyList()
            },
            heartRate = if (HealthPermission.HeartRate in permissions) {
                readAll(client, HeartRateRecord::class, range).flatMap { record ->
                    record.samples.mapIndexed { index, sample ->
                        HeartRateSample(
                            id = "${stableId(record.metadata)}:$index",
                            time = sample.time,
                            beatsPerMinute = sample.beatsPerMinute,
                            origin = deviceKind(record.metadata.device?.type) ?: "other",
                        )
                    }
                }
            } else {
                emptyList()
            },
        )
    }

    private suspend fun <T : Record> readAll(
        client: HealthConnectClient,
        type: KClass<T>,
        range: TimeRangeFilter,
    ): List<T> {
        val records = mutableListOf<T>()
        var pageToken: String? = null
        do {
            val response = client.readRecords(
                ReadRecordsRequest(
                    recordType = type,
                    timeRangeFilter = range,
                    ascendingOrder = true,
                    pageToken = pageToken,
                ),
            )
            records += response.records
            pageToken = response.pageToken
        } while (pageToken != null)
        return records
    }

    private fun stableId(metadata: Metadata): String {
        val id = metadata.id
        if (id.isNotBlank()) return id
        return metadata.clientRecordId?.takeIf { it.isNotBlank() }.orEmpty()
    }

    private fun sleepStageName(stage: Int): String = when (stage) {
        HealthSleepSession.STAGE_TYPE_AWAKE -> "awake"
        HealthSleepSession.STAGE_TYPE_SLEEPING -> "sleeping"
        HealthSleepSession.STAGE_TYPE_OUT_OF_BED -> "out_of_bed"
        HealthSleepSession.STAGE_TYPE_LIGHT -> "light"
        HealthSleepSession.STAGE_TYPE_DEEP -> "deep"
        HealthSleepSession.STAGE_TYPE_REM -> "rem"
        HealthSleepSession.STAGE_TYPE_AWAKE_IN_BED -> "awake_in_bed"
        else -> "unknown"
    }

    private fun discipline(type: Int): String = when (type) {
        HealthExerciseSession.EXERCISE_TYPE_RUNNING,
        HealthExerciseSession.EXERCISE_TYPE_RUNNING_TREADMILL,
        HealthExerciseSession.EXERCISE_TYPE_WALKING,
        HealthExerciseSession.EXERCISE_TYPE_HIKING,
        HealthExerciseSession.EXERCISE_TYPE_STAIR_CLIMBING,
        HealthExerciseSession.EXERCISE_TYPE_STAIR_CLIMBING_MACHINE
        -> "run"

        HealthExerciseSession.EXERCISE_TYPE_BIKING,
        HealthExerciseSession.EXERCISE_TYPE_BIKING_STATIONARY,
        HealthExerciseSession.EXERCISE_TYPE_ELLIPTICAL,
        HealthExerciseSession.EXERCISE_TYPE_ROWING,
        HealthExerciseSession.EXERCISE_TYPE_ROWING_MACHINE
        -> "ride"

        HealthExerciseSession.EXERCISE_TYPE_STRENGTH_TRAINING,
        HealthExerciseSession.EXERCISE_TYPE_WEIGHTLIFTING,
        HealthExerciseSession.EXERCISE_TYPE_CALISTHENICS,
        HealthExerciseSession.EXERCISE_TYPE_BOOT_CAMP,
        HealthExerciseSession.EXERCISE_TYPE_GYMNASTICS,
        HealthExerciseSession.EXERCISE_TYPE_ROCK_CLIMBING
        -> "strength"

        HealthExerciseSession.EXERCISE_TYPE_SWIMMING_OPEN_WATER,
        HealthExerciseSession.EXERCISE_TYPE_SWIMMING_POOL,
        HealthExerciseSession.EXERCISE_TYPE_WATER_POLO
        -> "swim"

        HealthExerciseSession.EXERCISE_TYPE_YOGA,
        HealthExerciseSession.EXERCISE_TYPE_PILATES,
        HealthExerciseSession.EXERCISE_TYPE_STRETCHING
        -> "mobility"

        else -> "other"
    }

    private fun deviceKind(type: Int?): String? = when (type) {
        Device.TYPE_WATCH -> "watch"
        Device.TYPE_PHONE -> "phone"
        Device.TYPE_SCALE -> "scale"
        Device.TYPE_RING -> "ring"
        Device.TYPE_HEAD_MOUNTED -> "head_mounted"
        Device.TYPE_FITNESS_BAND -> "fitness_band"
        Device.TYPE_CHEST_STRAP -> "chest_strap"
        Device.TYPE_SMART_DISPLAY -> "smart_display"
        else -> null
    }
}
