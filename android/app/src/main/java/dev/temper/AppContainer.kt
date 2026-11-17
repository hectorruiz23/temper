package dev.temper

import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat
import dev.temper.api.TemperApi
import dev.temper.config.ConfigStore
import dev.temper.config.StoredConfig
import dev.temper.intervals.IntervalRunnerService
import dev.temper.intervals.RunnerController
import dev.temper.intervals.RunnerReporter
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

class AppContainer(context: Context) {
    private val appContext = context.applicationContext
    val api = TemperApi()
    val configStore = ConfigStore(appContext)
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    val runnerController = RunnerController()
    val runnerReporter = RunnerReporter(api, scope)

    private val mutableConfiguration = MutableStateFlow(
        runBlocking(Dispatchers.IO) { configStore.read() },
    )
    val configuration: StateFlow<StoredConfig?> = mutableConfiguration.asStateFlow()

    suspend fun saveConfiguration(endpoint: String, token: String) {
        configStore.save(endpoint, token)
        mutableConfiguration.value = StoredConfig(endpoint, token)
    }

    suspend fun clearConfiguration() {
        configStore.clear()
        mutableConfiguration.value = null
    }

    fun startRunnerService() {
        ContextCompat.startForegroundService(
            appContext,
            Intent(appContext, IntervalRunnerService::class.java),
        )
    }

    fun pauseRunner() {
        appContext.startService(
            Intent(appContext, IntervalRunnerService::class.java).setAction(IntervalRunnerService.ACTION_PAUSE),
        )
    }

    fun resumeRunner() {
        appContext.startService(
            Intent(appContext, IntervalRunnerService::class.java).setAction(IntervalRunnerService.ACTION_RESUME),
        )
    }

    fun discardRunner() {
        appContext.startService(
            Intent(appContext, IntervalRunnerService::class.java).setAction(IntervalRunnerService.ACTION_DISCARD),
        )
    }
}
