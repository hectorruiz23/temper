package dev.temper.ui.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.temper.AppContainer
import dev.temper.api.ReadOutcome
import dev.temper.config.EndpointValidation
import dev.temper.config.normalizeEndpoint
import dev.temper.config.validateConfiguration
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

sealed interface ConnectionState {
    data object Idle : ConnectionState
    data object Testing : ConnectionState
    data class Passed(val message: String) : ConnectionState
    data class Failed(val message: String) : ConnectionState
}

sealed interface SaveState {
    data object Idle : SaveState
    data object Saving : SaveState
    data object Saved : SaveState
    data class Failed(val message: String) : SaveState
}

data class SettingsUiState(
    val endpoint: String = "",
    val token: String = "",
    val hasStoredToken: Boolean = false,
    val endpointError: String? = null,
    val tokenError: String? = null,
    val connection: ConnectionState = ConnectionState.Idle,
    val save: SaveState = SaveState.Idle,
)

class SettingsViewModel(private val container: AppContainer) : ViewModel() {
    private val stored = container.configuration.value
    private val mutableState = MutableStateFlow(
        SettingsUiState(
            endpoint = stored?.endpoint ?: "",
            hasStoredToken = stored != null,
        ),
    )
    val state: StateFlow<SettingsUiState> = mutableState.asStateFlow()

    fun onEndpointChange(value: String) {
        mutableState.update {
            it.copy(
                endpoint = value,
                endpointError = null,
                connection = ConnectionState.Idle,
                save = SaveState.Idle,
            )
        }
    }

    fun onTokenChange(value: String) {
        mutableState.update {
            it.copy(token = value, tokenError = null, connection = ConnectionState.Idle, save = SaveState.Idle)
        }
    }

    fun testConnection() {
        val endpoint = when (val validation = normalizeEndpoint(mutableState.value.endpoint)) {
            is EndpointValidation.Valid -> validation.endpoint
            is EndpointValidation.Invalid -> {
                mutableState.update { it.copy(endpointError = validation.message, connection = ConnectionState.Idle) }
                return
            }
        }
        mutableState.update {
            it.copy(endpoint = endpoint, endpointError = null, connection = ConnectionState.Testing)
        }
        viewModelScope.launch {
            when (val outcome = container.api.health(endpoint)) {
                is ReadOutcome.Success -> mutableState.update {
                    it.copy(connection = ConnectionState.Passed("The server answered. DNS, TLS and the endpoint are working."))
                }
                is ReadOutcome.Failure -> mutableState.update {
                    it.copy(connection = ConnectionState.Failed(outcome.problem.message))
                }
            }
        }
    }

    fun save() {
        val current = mutableState.value
        val validation = validateConfiguration(current.endpoint, current.token, container.configuration.value?.token)
        val endpoint = validation.endpoint
        val token = validation.token
        if (endpoint == null || token == null) {
            mutableState.update {
                it.copy(
                    endpointError = validation.endpointError,
                    tokenError = validation.tokenError,
                    save = SaveState.Idle,
                )
            }
            return
        }
        mutableState.update {
            it.copy(
                endpoint = endpoint,
                endpointError = null,
                tokenError = null,
                save = SaveState.Saving,
                connection = ConnectionState.Idle,
            )
        }
        viewModelScope.launch {
            when (val outcome = container.api.health(endpoint)) {
                is ReadOutcome.Failure -> mutableState.update {
                    it.copy(
                        save = SaveState.Failed(outcome.problem.message),
                        connection = ConnectionState.Failed(outcome.problem.message),
                    )
                }
                is ReadOutcome.Success -> {
                    container.saveConfiguration(endpoint, token)
                    mutableState.update {
                        it.copy(
                            token = "",
                            hasStoredToken = true,
                            save = SaveState.Saved,
                            connection = ConnectionState.Passed("The server answered. Configuration stored on this phone."),
                        )
                    }
                }
            }
        }
    }

    fun forget() {
        viewModelScope.launch {
            container.clearConfiguration()
            mutableState.value = SettingsUiState()
        }
    }
}
