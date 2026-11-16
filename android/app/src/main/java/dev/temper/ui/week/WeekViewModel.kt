package dev.temper.ui.week

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.temper.AppContainer
import dev.temper.api.ProblemKind
import dev.temper.api.ReadOutcome
import dev.temper.model.WeekResponse
import java.time.LocalDate
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

sealed interface WeekUiState {
    data object Loading : WeekUiState
    data object NeedsConfiguration : WeekUiState
    data class Content(val week: WeekResponse, val today: LocalDate) : WeekUiState
    data class Error(val message: String, val retryable: Boolean) : WeekUiState
}

class WeekViewModel(private val container: AppContainer) : ViewModel() {
    private val mutableState = MutableStateFlow<WeekUiState>(WeekUiState.Loading)
    val state: StateFlow<WeekUiState> = mutableState.asStateFlow()

    fun load() {
        val config = container.configuration.value
        if (config == null) {
            mutableState.value = WeekUiState.NeedsConfiguration
            return
        }
        mutableState.value = WeekUiState.Loading
        val today = LocalDate.now()
        viewModelScope.launch {
            when (val outcome = container.api.week(config.endpoint, config.token, today.toString())) {
                is ReadOutcome.Success -> mutableState.value = WeekUiState.Content(outcome.value, today)
                is ReadOutcome.Failure -> mutableState.value = WeekUiState.Error(
                    message = outcome.problem.message,
                    retryable = outcome.problem.kind == ProblemKind.Retryable || outcome.problem.kind == ProblemKind.Unknown,
                )
            }
        }
    }
}
