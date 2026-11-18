package dev.temper.health

import dev.temper.api.ProblemKind
import dev.temper.api.ReadOutcome
import dev.temper.api.SendOutcome
import dev.temper.api.TemperApi
import dev.temper.api.TemperJson
import dev.temper.config.ConfigStore
import dev.temper.config.StoredConfig
import dev.temper.model.DayMetrics
import dev.temper.model.DayUploadRequest
import dev.temper.model.DayWorkoutSplit
import dev.temper.model.HealthDay
import dev.temper.model.HealthDayEnvelope
import dev.temper.model.HealthSleepStage
import dev.temper.model.HealthWorkoutWire
import dev.temper.model.toDayWorkout
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

sealed interface HealthPhase {
    data object Idle : HealthPhase
    data object Checking : HealthPhase
    data object Syncing : HealthPhase
    data class RequestingPermissions(val missing: List<HealthPermission>) : HealthPhase
    data class Unavailable(val availability: HealthAvailability) : HealthPhase
    data class PermissionsDenied(val missing: List<HealthPermission>, val revoked: Boolean) : HealthPhase
    data class NoData(val dates: List<LocalDate>) : HealthPhase
    data class Uploaded(val synced: Int, val missing: List<LocalDate>, val withoutDistance: Int = 0) : HealthPhase
    data class Failed(val message: String, val retryable: Boolean, val requiresConfiguration: Boolean = false) : HealthPhase
}

data class HealthUiState(
    val phase: HealthPhase = HealthPhase.Idle,
    val lastSyncAt: Instant? = null,
    val permission: HealthPermissionState = HealthPermissionState.Unknown,
    val granted: Set<HealthPermission> = emptySet(),
    val missing: Set<HealthPermission> = emptySet(),
    val zone: ZoneId = ZoneId.systemDefault(),
)

class HealthSyncController(
    private val api: TemperApi,
    private val reader: HealthReader,
    private val configStore: ConfigStore,
    private val configuration: StateFlow<StoredConfig?>,
    private val scope: CoroutineScope,
) {
    private val mutableState = MutableStateFlow(HealthUiState())
    val state: StateFlow<HealthUiState> = mutableState.asStateFlow()

    private val mutex = Mutex()

    fun syncOnForeground() {
        scope.launch {
            if (!mutex.tryLock()) return@launch
            try {
                sync(interactive = false, requestPermissions = null)
            } finally {
                mutex.unlock()
            }
        }
    }

    fun syncNow(requestPermissions: (Set<String>) -> Unit) {
        scope.launch {
            if (!mutex.tryLock()) return@launch
            try {
                sync(interactive = true, requestPermissions = requestPermissions)
            } finally {
                mutex.unlock()
            }
        }
    }

    fun onPermissionsResult(granted: Set<String>) {
        val permissions = granted.mapNotNull { HealthPermission.fromPermission(it) }.toSet()
        scope.launch {
            mutex.withLock {
                rememberGranted(permissions)
                sync(interactive = false, requestPermissions = null)
            }
        }
    }

    private suspend fun sync(interactive: Boolean, requestPermissions: ((Set<String>) -> Unit)?) {
        val config = configuration.value
        if (config == null) {
            fail("No server endpoint or token is configured.", retryable = false, requiresConfiguration = true)
            return
        }

        val zone = ZoneId.systemDefault()
        update(HealthUiState(phase = HealthPhase.Checking, zone = zone))

        val availability = runCatching { reader.availability() }
            .getOrElse {
                fail("Health Connect could not be checked: ${it.message ?: "unknown error"}.", retryable = true)
                return
            }
        if (availability != HealthAvailability.Available) {
            finish(HealthUiState(phase = HealthPhase.Unavailable(availability), zone = zone))
            return
        }

        val granted = runCatching { reader.grantedPermissions() }
            .getOrElse {
                fail("Health Connect permissions could not be checked: ${it.message ?: "unknown error"}.", retryable = true)
                return
            }
        val previouslyGranted = readPreviouslyGranted()
        rememberGranted(granted)

        val missing = HealthPermission.all.filterNot { it in granted }.toSet()
        val permission = permissionState(granted, missing, previouslyGranted)
        val base = HealthUiState(
            phase = HealthPhase.Checking,
            permission = permission,
            granted = granted,
            missing = missing,
            zone = zone,
        )

        if (interactive && missing.isNotEmpty() && requestPermissions != null) {
            update(base.copy(phase = HealthPhase.RequestingPermissions(missing.sortedBy { it.ordinal })))
            withContext(Dispatchers.Main) {
                requestPermissions(missing.map { it.permission }.toSet())
            }
            return
        }

        if (granted.isEmpty()) {
            val revoked = missing.any { it.name in previouslyGranted }
            finish(
                base.copy(
                    phase = HealthPhase.PermissionsDenied(missing.sortedBy { it.ordinal }, revoked),
                ),
            )
            return
        }

        update(base.copy(phase = HealthPhase.Syncing))

        val today = LocalDate.now(zone)
        val syncedDays = mutableListOf<LocalDate>()
        val missingDays = mutableListOf<LocalDate>()
        var runsWithoutDistance = 0

        for (day in lastSevenDays(today)) {
            val records = runCatching { reader.read(day, zone, granted) }
                .getOrElse {
                    finish(
                        base.copy(
                            phase = HealthPhase.Failed(
                                "Health Connect could not be read: ${it.message ?: "unknown error"}.",
                                retryable = true,
                            ),
                        ),
                    )
                    return
                }
            val snapshot = HealthDayAggregator.aggregate(records, day, zone)
            if (!snapshot.hasData) {
                missingDays += day
                continue
            }
            when (val result = upload(config, snapshot, granted)) {
                is UploadResult.Current, is UploadResult.Saved -> {
                    syncedDays += day
                    runsWithoutDistance += snapshot.workouts.count { workout ->
                        workout.discipline == "run" && workout.distanceSource == WorkoutDistanceSource.Absent
                    }
                }
                is UploadResult.Failure -> {
                    finish(
                        base.copy(
                            phase = HealthPhase.Failed(result.message, result.retryable, result.requiresConfiguration),
                        ),
                    )
                    return
                }
            }
        }

        if (syncedDays.isEmpty()) {
            finish(base.copy(phase = HealthPhase.NoData(missingDays.sorted())))
        } else {
            finish(
                base.copy(
                    phase = HealthPhase.Uploaded(
                        synced = syncedDays.size,
                        missing = missingDays.sorted(),
                        withoutDistance = runsWithoutDistance,
                    ),
                ),
            )
        }
    }

    private suspend fun upload(
        config: StoredConfig,
        snapshot: HealthDaySnapshot,
        granted: Set<HealthPermission>,
    ): UploadResult {
        var conflictRetry = false
        while (true) {
            val existing = when (val outcome = api.day(config.endpoint, config.token, snapshot.date.toString())) {
                is ReadOutcome.Success -> outcome.value.day
                is ReadOutcome.Failure -> {
                    if (outcome.problem.status == 404) {
                        null
                    } else {
                        return UploadResult.Failure(
                            message = outcome.problem.message,
                            retryable = outcome.problem.kind == ProblemKind.Retryable ||
                                outcome.problem.kind == ProblemKind.Unknown,
                            requiresConfiguration = outcome.problem.status == 401,
                        )
                    }
                }
            }
            val expectedRevision = existing?.revision ?: 0L
            if (existing != null && matches(existing, snapshot, granted)) {
                return UploadResult.Current(existing.revision)
            }

            val body = TemperJson.encodeToString(requestFor(snapshot, expectedRevision, granted))
            val key = healthDayKey(snapshot.date, expectedRevision)
            when (val outcome = api.uploadDay(config.endpoint, config.token, key, body)) {
                is SendOutcome.Success -> {
                    val revision = runCatching {
                        TemperJson.decodeFromString<HealthDayEnvelope>(outcome.body).day.revision
                    }.getOrNull() ?: (expectedRevision + 1)
                    return UploadResult.Saved(revision)
                }
                is SendOutcome.Rejected -> {
                    if (outcome.code == "revision_conflict" && !conflictRetry) {
                        conflictRetry = true
                        continue
                    }
                    return UploadResult.Failure(
                        message = outcome.message,
                        retryable = false,
                        requiresConfiguration = outcome.status == 401,
                    )
                }
                is SendOutcome.Retryable -> return UploadResult.Failure(outcome.reason, retryable = true)
                is SendOutcome.Unknown -> return UploadResult.Failure(outcome.reason, retryable = false)
            }
        }
    }

    private fun requestFor(
        snapshot: HealthDaySnapshot,
        expectedRevision: Long,
        granted: Set<HealthPermission>,
    ): DayUploadRequest {
        val metrics = DayMetrics(
            sleepMs = snapshot.sleepMs,
            steps = snapshot.steps,
            restingHrBpm = snapshot.restingHrBpm,
            hrvRmssdMs = snapshot.hrvRmssdMs,
        ).takeUnless { it.isEmpty }

        return DayUploadRequest(
            date = snapshot.date.toString(),
            timezone = snapshot.timezone.id,
            expectedRevision = expectedRevision,
            source = "health_connect",
            metrics = metrics,
            sleepStages = if (HealthPermission.Sleep in granted) {
                snapshot.sleepStages.map { HealthSleepStage(it.stage, it.durationMs) }
            } else {
                null
            },
            workouts = if (HealthPermission.Exercise in granted) {
                snapshot.workouts.map { it.toDayWorkout() }
            } else {
                null
            },
        )
    }

    private fun matches(
        existing: HealthDay,
        snapshot: HealthDaySnapshot,
        granted: Set<HealthPermission>,
    ): Boolean {
        if (existing.timezone != snapshot.timezone.id) return false
        if (HealthPermission.Sleep in granted) {
            if (snapshot.sleepMs != null && existing.sleepMs != snapshot.sleepMs) return false
            if (existing.sleepStages.map { it.stage to it.durationMs }.sortedBy { it.first } !=
                snapshot.sleepStages.map { it.stage to it.durationMs }.sortedBy { it.first }
            ) {
                return false
            }
        }
        if (HealthPermission.Steps in granted && snapshot.steps != null && existing.steps != snapshot.steps) return false
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

    private suspend fun readPreviouslyGranted(): Set<String> =
        runCatching { configStore.readGrantedHealthPermissions() }.getOrDefault(emptySet())

    private suspend fun rememberGranted(granted: Set<HealthPermission>) {
        if (granted.isEmpty()) return
        val previous = readPreviouslyGranted()
        val merged = previous + granted.map { it.name }
        runCatching { configStore.saveGrantedHealthPermissions(merged) }
    }

    private fun permissionState(
        granted: Set<HealthPermission>,
        missing: Set<HealthPermission>,
        previouslyGranted: Set<String>,
    ): HealthPermissionState = when {
        granted.size == HealthPermission.all.size -> HealthPermissionState.Granted
        granted.isEmpty() && missing.any { it.name in previouslyGranted } -> HealthPermissionState.Revoked
        granted.isEmpty() -> HealthPermissionState.Denied
        missing.any { it.name in previouslyGranted } -> HealthPermissionState.Revoked
        else -> HealthPermissionState.Partial
    }

    private fun update(state: HealthUiState) {
        mutableState.value = state
    }

    private fun finish(state: HealthUiState) {
        mutableState.value = state.copy(lastSyncAt = Instant.now())
    }

    private fun fail(message: String, retryable: Boolean, requiresConfiguration: Boolean = false) {
        finish(
            mutableState.value.copy(
                phase = HealthPhase.Failed(message, retryable, requiresConfiguration),
            ),
        )
    }

    private sealed interface UploadResult {
        data class Current(val revision: Long) : UploadResult
        data class Saved(val revision: Long) : UploadResult
        data class Failure(
            val message: String,
            val retryable: Boolean,
            val requiresConfiguration: Boolean = false,
        ) : UploadResult
    }
}
