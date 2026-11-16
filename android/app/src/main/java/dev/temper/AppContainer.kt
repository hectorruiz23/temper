package dev.temper

import android.content.Context
import dev.temper.api.TemperApi
import dev.temper.config.ConfigStore
import dev.temper.config.StoredConfig
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
}
