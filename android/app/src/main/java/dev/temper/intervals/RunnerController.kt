package dev.temper.intervals

import dev.temper.runner.RunnerSnapshot
import dev.temper.runner.RunnerStep
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class RunnerLaunch(
    val target: String,
    val date: String,
    val templateId: String,
    val sessionId: String,
    val sessionRevision: Long,
    val startedAtMs: Long,
    val steps: List<RunnerStep>,
    val planVersionId: Long,
)

data class RunnerCompletion(
    val sessionId: String,
    val expectedRevision: Long,
    val completedAtMs: Long,
    val totalElapsedMs: Long,
    val stepsCompleted: Int,
    val stepCount: Int,
)

class RunnerController {
    private val mutableLaunch = MutableStateFlow<RunnerLaunch?>(null)
    val launch: StateFlow<RunnerLaunch?> = mutableLaunch.asStateFlow()

    private val mutableSnapshot = MutableStateFlow<RunnerSnapshot?>(null)
    val snapshot: StateFlow<RunnerSnapshot?> = mutableSnapshot.asStateFlow()

    private val mutableCompletion = MutableStateFlow<RunnerCompletion?>(null)
    val completion: StateFlow<RunnerCompletion?> = mutableCompletion.asStateFlow()

    fun prepare(launch: RunnerLaunch) {
        mutableLaunch.value = launch
        mutableSnapshot.value = null
        mutableCompletion.value = null
    }

    fun publish(snapshot: RunnerSnapshot) {
        mutableSnapshot.value = snapshot
    }

    fun complete(completion: RunnerCompletion) {
        mutableCompletion.value = completion
    }

    fun reset() {
        mutableLaunch.value = null
        mutableSnapshot.value = null
        mutableCompletion.value = null
    }
}
