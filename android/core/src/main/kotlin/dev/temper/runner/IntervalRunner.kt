package dev.temper.runner

import dev.temper.clock.DistanceSource
import dev.temper.clock.MonotonicClock
import dev.temper.clock.WallClock

enum class RunnerStatus {
    Ready,
    Running,
    Paused,
    Finished,
}

data class RunnerStep(
    val position: Int,
    val label: String,
    val durationMs: Long? = null,
    val distanceM: Double? = null,
) {
    val isTimed: Boolean get() = durationMs != null
    val isDistance: Boolean get() = distanceM != null
}

data class RunnerSnapshot(
    val steps: List<RunnerStep>,
    val stepIndex: Int,
    val status: RunnerStatus,
    val stepElapsedMs: Long,
    val stepRemainingMs: Long?,
    val stepDistanceDoneM: Double,
    val stepDistanceRemainingM: Double?,
    val runElapsedMs: Long,
    val distanceAvailable: Boolean,
    val distanceUnavailableForMs: Long,
    val finishedAtMs: Long?,
) {
    val currentStep: RunnerStep get() = steps[stepIndex]
    val nextStep: RunnerStep? get() = steps.getOrNull(stepIndex + 1)
    val stepNumber: Int get() = (stepIndex + 1).coerceAtMost(steps.size)
    val stepCount: Int get() = steps.size
    val finished: Boolean get() = status == RunnerStatus.Finished
}

data class RunnerTransition(
    val fromIndex: Int,
    val toIndex: Int,
    val label: String,
    val finished: Boolean,
)

data class RunnerTick(
    val snapshot: RunnerSnapshot,
    val transitions: List<RunnerTransition>,
)

class IntervalRunner(
    private val steps: List<RunnerStep>,
    private val monotonicClock: MonotonicClock,
    private val wallClock: WallClock,
    private val distanceSource: DistanceSource,
    private val minimumDistanceSampleM: Double = 1.0,
) {
    init {
        require(steps.isNotEmpty())
        require(steps.all { it.label.isNotBlank() })
        require(steps.all { step -> (step.durationMs != null && step.durationMs > 0) xor (step.distanceM != null && step.distanceM > 0) })
    }

    private var status = RunnerStatus.Ready
    private var index = 0
    private var stepElapsedMs = 0L
    private var stepRemainingMs = steps.first().durationMs ?: 0L
    private var stepDistanceM = 0.0
    private var runElapsedMs = 0L
    private var lastMonotonicMs = 0L
    private var distanceBaselineM: Double? = null
    private var distanceAvailable = true
    private var distanceUnavailableSinceRunMs: Long? = null
    private var finishedAtMs: Long? = null
    private val transitions = mutableListOf<RunnerTransition>()

    fun start(): List<RunnerTransition> {
        if (status != RunnerStatus.Ready) return emptyList()
        status = RunnerStatus.Running
        lastMonotonicMs = monotonicClock.elapsedRealtimeMs()
        if (steps.any { it.isDistance }) distanceSource.start()
        if (steps.first().isDistance) beginDistanceStep()
        return takeTransitions()
    }

    fun pause(): List<RunnerTransition> {
        if (status != RunnerStatus.Running) return emptyList()
        advanceTo(monotonicClock.elapsedRealtimeMs())
        if (status == RunnerStatus.Running) status = RunnerStatus.Paused
        return takeTransitions()
    }

    fun resume(): List<RunnerTransition> {
        if (status != RunnerStatus.Paused) return emptyList()
        status = RunnerStatus.Running
        lastMonotonicMs = monotonicClock.elapsedRealtimeMs()
        if (steps[index].isDistance) resetDistanceBaseline()
        return takeTransitions()
    }

    fun tick(): RunnerTick {
        if (status == RunnerStatus.Running) advanceTo(monotonicClock.elapsedRealtimeMs())
        return RunnerTick(snapshot(), takeTransitions())
    }

    fun snapshot(): RunnerSnapshot {
        val step = steps[index]
        val goal = step.distanceM
        val done = if (goal == null) 0.0 else stepDistanceM.coerceAtMost(goal)
        val distanceRemaining = if (goal == null) null else (goal - stepDistanceM).coerceAtLeast(0.0)
        val unavailableFor = distanceUnavailableSinceRunMs?.let { (runElapsedMs - it).coerceAtLeast(0L) } ?: 0L
        return RunnerSnapshot(
            steps = steps,
            stepIndex = index,
            status = status,
            stepElapsedMs = stepElapsedMs,
            stepRemainingMs = step.durationMs?.let { stepRemainingMs.coerceAtLeast(0L) },
            stepDistanceDoneM = done,
            stepDistanceRemainingM = distanceRemaining,
            runElapsedMs = runElapsedMs,
            distanceAvailable = distanceAvailable,
            distanceUnavailableForMs = unavailableFor,
            finishedAtMs = finishedAtMs,
        )
    }

    private fun advanceTo(now: Long) {
        if (status != RunnerStatus.Running) {
            lastMonotonicMs = now
            return
        }
        val delta = (now - lastMonotonicMs).coerceAtLeast(0L)
        lastMonotonicMs = now
        if (delta == 0L) {
            if (steps[index].isDistance) sampleDistance()
            return
        }
        var remaining = delta
        while (remaining > 0L && status == RunnerStatus.Running) {
            val step = steps[index]
            if (step.isTimed) {
                val consumed = minOf(remaining, stepRemainingMs)
                stepRemainingMs -= consumed
                stepElapsedMs += consumed
                runElapsedMs += consumed
                remaining -= consumed
                if (stepRemainingMs > 0L) break
                moveToNext()
                if (status == RunnerStatus.Finished) break
            } else {
                stepElapsedMs += remaining
                runElapsedMs += remaining
                remaining = 0L
                sampleDistance()
                while (status == RunnerStatus.Running && steps[index].isDistance && distanceReached()) {
                    moveToNext()
                    if (status == RunnerStatus.Finished) break
                    if (steps[index].isDistance) sampleDistance() else break
                }
            }
        }
    }

    private fun moveToNext() {
        val from = index
        if (index == steps.lastIndex) {
            status = RunnerStatus.Finished
            finishedAtMs = wallClock.nowMs()
            distanceSource.stop()
            transitions += RunnerTransition(from, index, steps[index].label, true)
            return
        }
        index += 1
        stepElapsedMs = 0L
        stepDistanceM = 0.0
        distanceBaselineM = null
        distanceAvailable = true
        distanceUnavailableSinceRunMs = null
        stepRemainingMs = steps[index].durationMs ?: 0L
        if (steps[index].isDistance) beginDistanceStep()
        transitions += RunnerTransition(from, index, steps[index].label, false)
    }

    private fun beginDistanceStep() {
        val reading = distanceSource.distanceMeters()
        if (reading == null) {
            if (distanceAvailable) distanceUnavailableSinceRunMs = runElapsedMs
            distanceAvailable = false
            distanceBaselineM = null
        } else {
            distanceBaselineM = reading
            distanceAvailable = true
            distanceUnavailableSinceRunMs = null
        }
    }

    private fun resetDistanceBaseline() {
        val reading = distanceSource.distanceMeters()
        if (reading == null) {
            if (distanceAvailable) distanceUnavailableSinceRunMs = runElapsedMs
            distanceAvailable = false
            distanceBaselineM = null
        } else {
            distanceBaselineM = reading
            distanceAvailable = true
            distanceUnavailableSinceRunMs = null
        }
    }

    private fun sampleDistance() {
        if (!steps[index].isDistance) return
        val reading = distanceSource.distanceMeters()
        if (reading == null) {
            if (distanceAvailable) distanceUnavailableSinceRunMs = runElapsedMs
            distanceAvailable = false
            return
        }
        distanceAvailable = true
        distanceUnavailableSinceRunMs = null
        val baseline = distanceBaselineM
        if (baseline == null) {
            distanceBaselineM = reading
            return
        }
        if (reading < baseline) {
            distanceBaselineM = reading
            return
        }
        val delta = reading - baseline
        val goal = steps[index].distanceM ?: Double.MAX_VALUE
        if (delta < minimumDistanceSampleM && stepDistanceM + delta < goal) return
        stepDistanceM += delta
        distanceBaselineM = reading
    }

    private fun distanceReached(): Boolean = stepDistanceM >= (steps[index].distanceM ?: Double.MAX_VALUE)

    private fun takeTransitions(): List<RunnerTransition> {
        if (transitions.isEmpty()) return emptyList()
        val taken = transitions.toList()
        transitions.clear()
        return taken
    }
}
