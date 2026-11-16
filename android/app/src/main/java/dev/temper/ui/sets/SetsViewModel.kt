package dev.temper.ui.sets

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.temper.AppContainer
import dev.temper.api.PendingOperation
import dev.temper.api.ProblemKind
import dev.temper.api.ReadOutcome
import dev.temper.api.TemperJson
import dev.temper.api.WriteState
import dev.temper.model.LogSet
import dev.temper.model.LogSetsRequest
import dev.temper.model.PlanExercise
import dev.temper.model.SetsEnvelope
import java.time.LocalDate
import java.util.UUID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SetRow(
    val rowKey: String,
    val exerciseId: String,
    val exerciseName: String,
    val setIndex: Int,
    val target: String,
    val kg: String,
    val reps: String,
    val note: String,
)

sealed interface SetsUiState {
    data object Loading : SetsUiState
    data object NeedsConfiguration : SetsUiState
    data class Content(val rows: List<SetRow>, val fieldError: String? = null) : SetsUiState
    data object Empty : SetsUiState
    data class Error(val message: String, val retryable: Boolean, val requiresConfiguration: Boolean = false) : SetsUiState
}

class SetsViewModel(
    private val container: AppContainer,
    private val sessionId: String,
    private val templateId: String,
    private val dateValue: String,
) : ViewModel() {
    private val mutableState = MutableStateFlow<SetsUiState>(SetsUiState.Loading)
    val state: StateFlow<SetsUiState> = mutableState.asStateFlow()

    private val mutableWriteState = MutableStateFlow<WriteState<SetsEnvelope>>(WriteState.Idle)
    val writeState: StateFlow<WriteState<SetsEnvelope>> = mutableWriteState.asStateFlow()

    private var operation: PendingOperation<SetsEnvelope>? = null

    init {
        load()
    }

    fun load() {
        val config = container.configuration.value
        if (config == null) {
            mutableState.value = SetsUiState.NeedsConfiguration
            return
        }
        mutableState.value = SetsUiState.Loading
        viewModelScope.launch {
            val plan = when (val outcome = container.api.currentPlan(config.endpoint, config.token)) {
                is ReadOutcome.Success -> outcome.value.plan
                is ReadOutcome.Failure -> {
                    mutableState.value = error(outcome.problem.message, outcome.problem.kind, outcome.problem.status)
                    return@launch
                }
            }
            val exercises = when (val outcome = container.api.exercises(config.endpoint, config.token)) {
                is ReadOutcome.Success -> outcome.value.exercises
                is ReadOutcome.Failure -> {
                    mutableState.value = error(outcome.problem.message, outcome.problem.kind, outcome.problem.status)
                    return@launch
                }
            }
            val template = plan.sessions.firstOrNull { it.id == templateId }
            if (template == null || template.exercises.isEmpty()) {
                mutableState.value = SetsUiState.Empty
                return@launch
            }
            val names = exercises.associate { it.id to it.name }
            val rows = template.exercises.sortedBy { it.position }.flatMap { exercise ->
                val count = exercise.targetSets ?: 1
                (1..count).map { setIndex ->
                    SetRow(
                        rowKey = "${exercise.exerciseId}-$setIndex",
                        exerciseId = exercise.exerciseId,
                        exerciseName = names[exercise.exerciseId] ?: exercise.exerciseId,
                        setIndex = setIndex,
                        target = targetLabel(exercise),
                        kg = exercise.targetKg?.let(::numberText).orEmpty(),
                        reps = (exercise.repsMin ?: exercise.repsMax ?: 1).toString(),
                        note = "",
                    )
                }
            }
            mutableState.value = SetsUiState.Content(rows)
        }
    }

    fun onKgChange(rowKey: String, value: String) {
        if (mutableWriteState.value is WriteState.Sending || mutableWriteState.value is WriteState.Saved) return
        updateRow(rowKey) { it.copy(kg = value) }
    }

    fun onRepsChange(rowKey: String, value: String) {
        if (mutableWriteState.value is WriteState.Sending || mutableWriteState.value is WriteState.Saved) return
        updateRow(rowKey) { it.copy(reps = value) }
    }

    fun onNoteChange(rowKey: String, value: String) {
        if (mutableWriteState.value is WriteState.Sending || mutableWriteState.value is WriteState.Saved) return
        updateRow(rowKey) { it.copy(note = value) }
    }

    fun submit() {
        val content = mutableState.value as? SetsUiState.Content ?: return
        val config = container.configuration.value ?: return
        if (mutableWriteState.value is WriteState.Sending) return
        val parsed = parseRows(content.rows)
        if (parsed.error != null || parsed.sets == null) {
            mutableState.value = content.copy(fieldError = parsed.error)
            return
        }
        val request = LogSetsRequest(sessionId = sessionId, sets = parsed.sets)
        val body = TemperJson.encodeToString(request)
        mutableState.value = content.copy(fieldError = null)
        viewModelScope.launch {
            val existing = operation
            val current = existing?.state?.value
            val reuse = existing != null &&
                existing.body == body &&
                (current is WriteState.Retryable || current is WriteState.UnknownOutcome)
            val next = if (reuse) existing else PendingOperation(
                key = UUID.randomUUID().toString(),
                body = body,
                send = { key, payload -> container.api.logSets(config.endpoint, config.token, key, payload) },
                decode = { payload -> TemperJson.decodeFromString<SetsEnvelope>(payload) },
            )
            operation = next
            mutableWriteState.value = WriteState.Sending
            if (reuse) next.retry() else next.submit()
            mutableWriteState.value = next.state.value
        }
    }

    fun retry() {
        val existing = operation ?: return
        val current = existing.state.value
        if (current !is WriteState.Retryable && current !is WriteState.UnknownOutcome) return
        viewModelScope.launch {
            mutableWriteState.value = WriteState.Sending
            existing.retry()
            mutableWriteState.value = existing.state.value
        }
    }

    private fun updateRow(rowKey: String, transform: (SetRow) -> SetRow) {
        val content = mutableState.value as? SetsUiState.Content ?: return
        mutableState.value = content.copy(
            rows = content.rows.map { if (it.rowKey == rowKey) transform(it) else it },
            fieldError = null,
        )
        operation = null
        mutableWriteState.value = WriteState.Idle
    }

    private fun parseRows(rows: List<SetRow>): ParsedRows {
        val sets = ArrayList<LogSet>(rows.size)
        for (row in rows) {
            val kg = row.kg.trim().replace(',', '.').toDoubleOrNull()
            if (kg == null || kg < 0) return ParsedRows(error = "Kg must be a non-negative number. Check ${row.exerciseName} set ${row.setIndex}.")
            val reps = row.reps.trim().toIntOrNull()
            if (reps == null || reps <= 0) return ParsedRows(error = "Reps must be greater than zero. Check ${row.exerciseName} set ${row.setIndex}.")
            sets += LogSet(
                exerciseId = row.exerciseId,
                setIndex = row.setIndex,
                kg = kg,
                reps = reps,
                note = row.note.trim().takeIf { it.isNotEmpty() },
            )
        }
        return ParsedRows(sets = sets)
    }

    private fun error(message: String, kind: ProblemKind, status: Int?): SetsUiState =
        SetsUiState.Error(
            message = message,
            retryable = kind == ProblemKind.Retryable || kind == ProblemKind.Unknown,
            requiresConfiguration = status == 401,
        )

    private fun targetLabel(exercise: PlanExercise): String {
        val sets = exercise.targetSets
        val reps = when {
            exercise.repsMin != null && exercise.repsMax != null && exercise.repsMin != exercise.repsMax ->
                "${exercise.repsMin}–${exercise.repsMax}"
            exercise.repsMin != null -> exercise.repsMin.toString()
            exercise.repsMax != null -> exercise.repsMax.toString()
            else -> "?"
        }
        val base = if (sets != null) "$sets×$reps" else "$reps reps"
        val kg = exercise.targetKg?.let { " · ${numberText(it)} kg" } ?: ""
        val tempo = exercise.tempo?.let { " · tempo $it" } ?: ""
        return base + kg + tempo
    }

    private fun numberText(value: Double): String =
        if (value % 1.0 == 0.0) value.toInt().toString() else value.toString()

    private data class ParsedRows(val sets: List<LogSet>? = null, val error: String? = null)
}
