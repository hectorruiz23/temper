package dev.temper.ui.session

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.api.WriteState
import dev.temper.model.SessionEnvelope
import dev.temper.ui.TemperSimpleFactory
import dev.temper.ui.components.AccentBlock
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
import dev.temper.ui.components.TemperTopBar
import dev.temper.ui.longDayLabel
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel

@Composable
fun SessionScreen(
    container: AppContainer,
    date: String,
    templateId: String,
    onBack: () -> Unit,
    onOpenSettings: () -> Unit,
    onStarted: (sessionId: String, revision: Long) -> Unit,
) {
    val viewModel = viewModel<SessionViewModel>(
        factory = TemperSimpleFactory { SessionViewModel(container, date, templateId) },
    )
    val state by viewModel.state.collectAsState()
    val write by viewModel.writeState.collectAsState()
    val content = (state as? SessionUiState.Content)?.content
    val savedSession = (write as? WriteState.Saved<SessionEnvelope>)?.value?.session

    LaunchedEffect(write) {
        val saved = write as? WriteState.Saved<SessionEnvelope>
        if (saved != null) onStarted(saved.value.session.id, saved.value.session.revision)
    }

    val openSession: () -> Unit = {
        val current = content
        val existingId = current?.existingSessionId ?: savedSession?.id
        if (existingId != null) {
            val revision = if (current?.existingSessionId != null) current.existingRevision else savedSession?.revision ?: 0L
            onStarted(existingId, revision)
        } else {
            viewModel.start()
        }
    }

    TemperScaffold(
        topBar = {
            TemperTopBar(
                title = content?.target?.ifBlank { "Session" } ?: "Session",
                subtitle = content?.let { "${longDayLabel(it.date)} · plan v${it.planVersion}" },
                onBack = onBack,
            )
        },
        rail = {
            when {
                content != null && write is WriteState.Sending -> {
                    RailHint("Creating the session", "in flight")
                    PrimaryButton(text = "Opening…", onClick = {}, enabled = false)
                }
                content != null && (write is WriteState.Retryable || write is WriteState.UnknownOutcome) -> {
                    RailHint("Not started", "retry uses the same key")
                    PrimaryButton(text = "Retry open", onClick = viewModel::retry)
                }
                content != null && write is WriteState.Rejected -> {
                    RailHint("Rejected by the server", null)
                    PrimaryButton(text = "Open session", onClick = openSession)
                    QuietButton(text = "Open Settings", onClick = onOpenSettings)
                }
                content != null -> {
                    RailHint(
                        "${content.exercises.size} exercises",
                        if (content.existingSessionId != null || savedSession != null) "session exists" else "session not created",
                    )
                    PrimaryButton(text = "Open session", onClick = openSession)
                }
                state is SessionUiState.NeedsConfiguration -> {
                    RailHint("No configuration", null)
                    PrimaryButton(text = "Open Settings", onClick = onOpenSettings)
                }
                state is SessionUiState.Error -> RailHint("Nothing started", null)
                else -> RailHint("Reading the plan", null)
            }
        },
    ) {
        when (val current = state) {
            is SessionUiState.Loading -> LoadingPane()
            is SessionUiState.NeedsConfiguration -> EmptyPane(
                eyebrow = "Configuration",
                title = "No server yet",
                detail = "Open Settings before opening a session.",
                action = "Open Settings",
                onAction = onOpenSettings,
            )
            is SessionUiState.Empty -> EmptyPane(
                eyebrow = "Session",
                title = "Session not found",
                detail = current.message,
                action = "Back to the week",
                onAction = onBack,
            )
            is SessionUiState.Error -> ErrorPane(
                title = "The session detail did not arrive",
                detail = current.message,
                action = when {
                    current.requiresConfiguration -> "Open Settings"
                    current.retryable -> "Try again"
                    else -> "Back to the week"
                },
                onAction = when {
                    current.requiresConfiguration -> onOpenSettings
                    current.retryable -> viewModel::load
                    else -> onBack
                },
            )
            is SessionUiState.Content -> SessionContentPane(content = current.content, write = write)
        }
    }
}

@Composable
private fun SessionContentPane(content: SessionContent, write: WriteState<SessionEnvelope>) {
    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 16.dp),
    ) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Eyebrow("${longDayLabel(content.date)} · ${content.discipline}")
                Text(
                    text = "${content.exercises.size} exercises · plan v${content.planVersion}",
                    color = Panel,
                    style = MaterialTheme.typography.labelMedium,
                )
            }
            Spacer(modifier = Modifier.height(8.dp))
            WriteStrip(write = write)
        }
        items(content.exercises) { exercise ->
            Row(
                modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
                verticalAlignment = Alignment.Top,
            ) {
                Text(
                    text = (content.exercises.indexOf(exercise) + 1).toString().padStart(2, '0'),
                    color = Panel,
                    style = MaterialTheme.typography.labelMedium,
                    modifier = Modifier.width(34.dp),
                )
                Column(modifier = Modifier.weight(1f)) {
                    Text(text = exercise.name, color = Ink, style = MaterialTheme.typography.titleLarge)
                    Text(text = exercise.target, color = Panel, style = MaterialTheme.typography.labelMedium)
                }
            }
            Hairline()
        }
        item {
            Spacer(modifier = Modifier.height(12.dp))
            AccentBlock {
                val ready = write is WriteState.Saved || content.existingSessionId != null
                Eyebrow(if (ready) "Ready" else "Before you start")
                Text(
                    text = if (ready) "Session ready" else "Open session creates the record",
                    color = Ink,
                    style = MaterialTheme.typography.headlineMedium,
                )
                Text(
                    text = if (ready) {
                        "Opening continues to the existing session and its set logging."
                    } else {
                        "The session is created before the first set. Only the check-in marks it complete."
                    },
                    color = Ink,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
    }
}

@Composable
private fun WriteStrip(write: WriteState<SessionEnvelope>) {
    when (write) {
        is WriteState.Retryable -> SyncStrip(
            mark = "✗",
            title = "Not started",
            detail = "${write.reason} Nothing was created.",
        )
        is WriteState.UnknownOutcome -> SyncStrip(
            mark = "!",
            title = "No answer from the server",
            detail = "${write.reason} The session may exist. Retrying uses the same key.",
            dark = true,
        )
        is WriteState.Rejected -> SyncStrip(
            mark = "✗",
            title = "The server rejected the session",
            detail = "${write.message} (${write.code ?: "HTTP ${write.status}"})",
        )
        WriteState.Sending -> SyncStrip(
            mark = "…",
            title = "Creating the session",
            detail = "Waiting for the server. Inputs are locked until it answers.",
            dashed = true,
        )
        else -> Unit
    }
}
