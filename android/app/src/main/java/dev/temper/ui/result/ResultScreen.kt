package dev.temper.ui.result

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.api.WriteState
import dev.temper.model.CheckInEnvelope
import dev.temper.ui.TemperSimpleFactory
import dev.temper.ui.components.AccentBlock
import dev.temper.ui.components.EmptyPane
import dev.temper.ui.components.ErrorPane
import dev.temper.ui.components.Eyebrow
import dev.temper.ui.components.Hairline
import dev.temper.ui.components.LoadingPane
import dev.temper.ui.components.MonoText
import dev.temper.ui.components.PrimaryButton
import dev.temper.ui.components.QuietButton
import dev.temper.ui.components.RailHint
import dev.temper.ui.components.SyncStrip
import dev.temper.ui.components.TemperScaffold
import dev.temper.ui.components.TemperTopBar
import dev.temper.ui.longDayLabel
import dev.temper.ui.parseDate
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel

@Composable
fun ResultScreen(
    container: AppContainer,
    sessionId: String,
    date: String,
    revision: Long,
    onBack: () -> Unit,
    onOpenSettings: () -> Unit,
    onDone: () -> Unit,
) {
    val viewModel = viewModel<ResultViewModel>(
        factory = TemperSimpleFactory { ResultViewModel(container, sessionId, date, revision) },
    )
    val state by viewModel.state.collectAsState()
    val write by viewModel.writeState.collectAsState()
    val content = state as? ResultUiState.Content
    val parsedDate = parseDate(date)

    TemperScaffold(
        topBar = {
            TemperTopBar(
                title = "Result",
                subtitle = parsedDate?.let { longDayLabel(it) } ?: "Session",
                onBack = onBack,
            )
        },
        rail = {
            when {
                write is WriteState.Saved -> {
                    RailHint("Session closed", "evidence recorded")
                    PrimaryButton(text = "Back to the week", onClick = onDone)
                }
                write is WriteState.Sending -> {
                    RailHint("Sending the check-in", "in flight")
                    PrimaryButton(text = "Closing…", onClick = {}, enabled = false)
                }
                write is WriteState.Retryable || write is WriteState.UnknownOutcome -> {
                    RailHint("Not confirmed", "retry uses the same key")
                    PrimaryButton(text = "Retry", onClick = viewModel::retry)
                }
                write is WriteState.Rejected -> {
                    RailHint("Rejected by the server", null)
                    PrimaryButton(text = "Mark complete", onClick = viewModel::markComplete)
                    QuietButton(text = "Back to the week", onClick = onDone)
                    QuietButton(text = "Open Settings", onClick = onOpenSettings)
                }
                content != null -> {
                    RailHint("Finish at ${java.time.LocalTime.now().withNano(0)}", "owner's word")
                    PrimaryButton(text = "Mark complete", onClick = viewModel::markComplete)
                    if (content.watchWorkoutId != null) {
                        QuietButton(text = "Link watch workout", onClick = viewModel::linkWatch)
                    }
                }
                state is ResultUiState.NeedsConfiguration -> {
                    RailHint("No configuration", null)
                    PrimaryButton(text = "Open Settings", onClick = onOpenSettings)
                }
                else -> RailHint("Reading the session", null)
            }
        },
    ) {
        when (val current = state) {
            is ResultUiState.Loading -> LoadingPane()
            is ResultUiState.NeedsConfiguration -> EmptyPane(
                eyebrow = "Configuration",
                title = "No server yet",
                detail = "Open Settings before closing a session.",
                action = "Open Settings",
                onAction = onOpenSettings,
            )
            is ResultUiState.Error -> ErrorPane(
                title = "The week did not arrive",
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
            is ResultUiState.Content -> ResultContent(
                content = current,
                revision = revision,
                sessionId = sessionId,
                write = write,
                onDone = onDone,
            )
        }
    }
}

@Composable
private fun ResultContent(
    content: ResultUiState.Content,
    revision: Long,
    sessionId: String,
    write: WriteState<CheckInEnvelope>,
    onDone: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        AccentBlock {
            Eyebrow(if (write is WriteState.Saved) "Closed" else "All sets are in")
            Text(
                text = if (write is WriteState.Saved) "Session complete" else "Session complete?",
                color = Ink,
                style = MaterialTheme.typography.headlineLarge,
            )
            Text(
                text = "Closing is your word. It marks the session complete with the time the server receives.",
                color = Ink,
                style = MaterialTheme.typography.bodyMedium,
            )
        }

        when (write) {
            is WriteState.Saved -> {
                val evidence = write.value.effectiveEvidence
                SyncStrip(
                    mark = "✓",
                    title = "Check-in saved",
                    detail = "Evidence: ${evidence?.source ?: "recorded"} · revision ${write.value.session.revision}",
                )
                MonoText(text = "session ${write.value.session.id}", color = Panel, size = 13)
                QuietButton(text = "Back to the week", onClick = onDone)
            }
            is WriteState.Retryable -> SyncStrip(mark = "✗", title = "Not sent", detail = "${write.reason} Nothing was recorded.")
            is WriteState.UnknownOutcome -> SyncStrip(
                mark = "!",
                title = "The check-in was not confirmed",
                detail = "${write.reason} The session may already be closed.",
                dark = true,
            )
            is WriteState.Rejected -> SyncStrip(
                mark = "✗",
                title = "The server rejected the check-in",
                detail = "${write.message} (${write.code ?: "HTTP ${write.status}"})",
            )
            WriteState.Sending -> SyncStrip(
                mark = "…",
                title = "Closing the session",
                detail = "Waiting for the server to confirm the check-in.",
                dashed = true,
            )
            else -> Unit
        }

        Hairline()

        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Eyebrow("Evidence")
            Text(
                text = if (content.watchWorkoutId == null) {
                    "No watch workout is linked to this date. Mark complete uses the time you tap it."
                } else {
                    "A watch workout is available on this date. Link it or mark complete, never both."
                },
                color = Ink,
                style = MaterialTheme.typography.bodyLarge,
            )
        }

        Hairline()
        MonoText(text = "session $sessionId", color = Panel, size = 13)
        MonoText(text = "expected revision $revision", color = Panel, size = 13)

        Text(
            text = "Closing now is final. To leave the session open, go back without marking it complete; you can finish it later.",
            color = Panel,
            style = MaterialTheme.typography.labelMedium,
        )
        Spacer(modifier = Modifier.height(8.dp))
    }
}
