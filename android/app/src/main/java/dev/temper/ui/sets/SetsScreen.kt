package dev.temper.ui.sets

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.api.WriteState
import dev.temper.model.SetsEnvelope
import dev.temper.ui.TemperSimpleFactory
import dev.temper.ui.components.EmptyPane
import dev.temper.ui.components.ErrorPane
import dev.temper.ui.components.Eyebrow
import dev.temper.ui.components.Hairline
import dev.temper.ui.components.LoadingPane
import dev.temper.ui.components.PrimaryButton
import dev.temper.ui.components.QuietButton
import dev.temper.ui.components.RailHint
import dev.temper.ui.components.SyncStrip
import dev.temper.ui.components.TemperScaffold
import dev.temper.ui.components.TemperTextField
import dev.temper.ui.components.TemperTopBar
import dev.temper.ui.formatSet
import dev.temper.ui.longDayLabel
import dev.temper.ui.parseDate
import dev.temper.ui.theme.Card
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel

@Composable
fun SetsScreen(
    container: AppContainer,
    sessionId: String,
    templateId: String,
    date: String,
    revision: Long,
    onBack: () -> Unit,
    onOpenSettings: () -> Unit,
    onContinue: () -> Unit,
) {
    val viewModel = viewModel<SetsViewModel>(
        factory = TemperSimpleFactory { SetsViewModel(container, sessionId, templateId, date) },
    )
    val state by viewModel.state.collectAsState()
    val write by viewModel.writeState.collectAsState()
    val content = state as? SetsUiState.Content
    val parsedDate = parseDate(date)

    TemperScaffold(
        topBar = {
            TemperTopBar(
                title = "Log sets",
                subtitle = parsedDate?.let { longDayLabel(it) } ?: "Session",
                onBack = onBack,
            )
        },
        rail = {
            when {
                write is WriteState.Saved -> {
                    RailHint("Saved on the server", "batch echoed")
                    PrimaryButton(text = "Continue to result", onClick = onContinue)
                }
                write is WriteState.Sending -> {
                    RailHint("Sending the batch", "inputs locked")
                    PrimaryButton(text = "Saving…", onClick = {}, enabled = false)
                }
                write is WriteState.Retryable || write is WriteState.UnknownOutcome -> {
                    RailHint("Not saved", "retry uses the same key")
                    PrimaryButton(text = "Retry", onClick = viewModel::retry)
                }
                write is WriteState.Rejected -> {
                    RailHint("Rejected by the server", null)
                    PrimaryButton(text = "Save sets", onClick = viewModel::submit)
                    QuietButton(text = "Open Settings", onClick = onOpenSettings)
                }
                else -> {
                    RailHint("${content?.rows?.size ?: 0} sets · one batch", "server truth only")
                    PrimaryButton(text = "Save sets", onClick = viewModel::submit, enabled = content != null)
                }
            }
        },
    ) {
        when (val current = state) {
            is SetsUiState.Loading -> LoadingPane()
            is SetsUiState.NeedsConfiguration -> EmptyPane(
                eyebrow = "Configuration",
                title = "No server yet",
                detail = "Open Settings before logging sets.",
                action = "Open Settings",
                onAction = onOpenSettings,
            )
            is SetsUiState.Empty -> EmptyPane(
                eyebrow = "Session",
                title = "No sets to log",
                detail = "This plan session has no exercises.",
                action = "Back",
                onAction = onBack,
            )
            is SetsUiState.Error -> ErrorPane(
                title = "The exercise list did not arrive",
                detail = current.message,
                action = when {
                    current.requiresConfiguration -> "Open Settings"
                    current.retryable -> "Try again"
                    else -> "Back"
                },
                onAction = when {
                    current.requiresConfiguration -> onOpenSettings
                    current.retryable -> viewModel::load
                    else -> onBack
                },
            )
            is SetsUiState.Content -> SetsContentPane(
                content = current,
                write = write,
                onKgChange = viewModel::onKgChange,
                onRepsChange = viewModel::onRepsChange,
                onNoteChange = viewModel::onNoteChange,
            )
        }
    }
}

@Composable
private fun SetsContentPane(
    content: SetsUiState.Content,
    write: WriteState<SetsEnvelope>,
    onKgChange: (String, String) -> Unit,
    onRepsChange: (String, String) -> Unit,
    onNoteChange: (String, String) -> Unit,
) {
    val locked = write is WriteState.Sending || write is WriteState.Saved
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        item {
            WriteStrip(write)
            if (content.fieldError != null) {
                SyncStrip(mark = "✗", title = "Check the numbers", detail = content.fieldError)
            }
        }
        items(content.rows, key = { it.rowKey }) { row ->
            val firstOfExercise = content.rows.firstOrNull { it.exerciseId == row.exerciseId }?.rowKey == row.rowKey
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(Card)
                    .padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                if (firstOfExercise) {
                    Eyebrow("${row.exerciseName} · ${row.target}")
                }
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text(
                        text = "Set ${row.setIndex}",
                        color = Panel,
                        style = MaterialTheme.typography.labelMedium,
                        modifier = Modifier.width(54.dp),
                    )
                    TemperTextField(
                        value = row.kg,
                        onValueChange = { onKgChange(row.rowKey, it) },
                        label = "kg",
                        enabled = !locked,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        modifier = Modifier.weight(1f),
                        minHeight = 60.dp,
                    )
                    TemperTextField(
                        value = row.reps,
                        onValueChange = { onRepsChange(row.rowKey, it) },
                        label = "reps",
                        enabled = !locked,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.weight(1f),
                        minHeight = 60.dp,
                    )
                }
                TemperTextField(
                    value = row.note,
                    onValueChange = { onNoteChange(row.rowKey, it) },
                    label = "Note (optional)",
                    enabled = !locked,
                    singleLine = false,
                    minHeight = 56.dp,
                )
            }
        }
    }
}

@Composable
private fun WriteStrip(write: WriteState<SetsEnvelope>) {
    when (write) {
        is WriteState.Saved -> {
            val echo = write.value.echo.sets
            val summary = echo.groupBy { it.exerciseId }.entries.joinToString(" · ") { (exercise, sets) ->
                "$exercise ${sets.joinToString(", ") { formatSet(it.kg, it.reps) }}"
            }.ifBlank { "The server confirmed the batch." }
            SyncStrip(
                mark = "✓",
                title = "Sets saved",
                detail = summary,
            )
        }
        is WriteState.Retryable -> SyncStrip(
            mark = "✗",
            title = "Not sent",
            detail = "${write.reason} Your numbers are still here.",
        )
        is WriteState.UnknownOutcome -> SyncStrip(
            mark = "!",
            title = "No answer from the server",
            detail = "${write.reason} Retrying sends the same numbers under the same key.",
            dark = true,
        )
        is WriteState.Rejected -> SyncStrip(
            mark = "✗",
            title = "The server rejected the sets",
            detail = "${write.message} (${write.code ?: "HTTP ${write.status}"})",
        )
        WriteState.Sending -> SyncStrip(
            mark = "…",
            title = "Sending the batch",
            detail = "The fields hold still until the server answers.",
            dashed = true,
        )
        else -> Unit
    }
    Spacer(modifier = Modifier.height(4.dp))
}
