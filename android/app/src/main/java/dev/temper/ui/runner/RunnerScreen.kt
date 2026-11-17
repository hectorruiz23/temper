package dev.temper.ui.runner

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.api.WriteState
import dev.temper.model.SessionEnvelope
import dev.temper.runner.RunnerSnapshot
import dev.temper.runner.RunnerStatus
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
import dev.temper.ui.theme.Action
import dev.temper.ui.theme.DataFont
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel
import dev.temper.ui.theme.PaperSoft
import java.util.Locale

@Composable
fun RunnerScreen(
    container: AppContainer,
    date: String,
    templateId: String,
    planVersionId: Long,
    onBack: () -> Unit,
    onOpenSettings: () -> Unit,
    onDone: () -> Unit,
) {
    val viewModel = viewModel<RunnerViewModel>(
        factory = TemperSimpleFactory { RunnerViewModel(container, date, templateId, planVersionId) },
    )
    val state by viewModel.state.collectAsState()
    val create by viewModel.createState.collectAsState()
    val snapshot by viewModel.run.collectAsState()
    val report by viewModel.report.collectAsState()
    val completion by viewModel.completion.collectAsState()
    val content = (state as? RunnerUiState.Content)?.content
    val context = LocalContext.current
    var exitPrompt by remember { mutableStateOf(false) }

    val permissions = remember(content?.distanceRequired) {
        buildList {
            if (Build.VERSION.SDK_INT >= 33 &&
                ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
            ) {
                add(Manifest.permission.POST_NOTIFICATIONS)
            }
            if (content?.distanceRequired == true) {
                if (ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
                    add(Manifest.permission.ACCESS_FINE_LOCATION)
                }
                if (ContextCompat.checkSelfPermission(context, Manifest.permission.ACCESS_COARSE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
                    add(Manifest.permission.ACCESS_COARSE_LOCATION)
                }
            }
        }
    }
    val permissionLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        viewModel.start()
    }
    val beginRun: () -> Unit = {
        if (permissions.isEmpty()) viewModel.start() else permissionLauncher.launch(permissions.toTypedArray())
    }

    val running = snapshot?.status == RunnerStatus.Running
    val paused = snapshot?.status == RunnerStatus.Paused
    BackHandler(enabled = running || paused) {
        if (running) container.pauseRunner()
        exitPrompt = true
    }

    if (exitPrompt) {
        AlertDialog(
            onDismissRequest = { exitPrompt = false },
            title = { Text("Run paused", color = Ink) },
            text = {
                Text(
                    "The runner keeps the session in the foreground service. Resume now, or end the run without reporting a completion.",
                    color = Ink,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    exitPrompt = false
                    container.resumeRunner()
                }) { Text("Resume", color = Ink) }
            },
            dismissButton = {
                TextButton(onClick = {
                    exitPrompt = false
                    viewModel.discardRun()
                    onBack()
                }) { Text("End without reporting", color = Panel) }
            },
        )
    }

    val subtitle = when {
        snapshot != null -> "$date · step ${snapshot!!.stepNumber} of ${snapshot!!.stepCount}"
        content != null -> "${content.date} · ${content.steps.size} steps · plan v${content.planVersionId}"
        else -> date
    }

    TemperScaffold(
        topBar = {
            TemperTopBar(
                title = content?.target ?: "Intervals",
                subtitle = subtitle,
                onBack = if (running || paused) null else {
                    {
                        viewModel.closeFinished()
                        onBack()
                    }
                },
            )
        },
        rail = {
            when {
                snapshot != null -> RunnerRail(
                    snapshot = snapshot!!,
                    report = report,
                    onPause = container::pauseRunner,
                    onResume = container::resumeRunner,
                    onDiscard = {
                        viewModel.discardRun()
                        onBack()
                    },
                    onRetryReport = viewModel::retryReport,
                    onReport = viewModel::reportNow,
                    onDone = {
                        viewModel.closeFinished()
                        onDone()
                    },
                    onOpenSettings = onOpenSettings,
                )
                state is RunnerUiState.NeedsConfiguration -> {
                    RailHint("No endpoint or token", "open Settings")
                    PrimaryButton(text = "Open Settings", onClick = onOpenSettings)
                }
                state is RunnerUiState.Error -> {
                    RailHint("The plan did not arrive", null)
                    PrimaryButton(
                        text = if ((state as RunnerUiState.Error).retryable) "Try again" else "Back to the week",
                        onClick = if ((state as RunnerUiState.Error).retryable) viewModel::load else onBack,
                    )
                }
                state is RunnerUiState.Empty -> {
                    RailHint("No interval steps", "nothing to run")
                    QuietButton(text = "Back to the week", onClick = onBack)
                }
                content != null && create is WriteState.Sending -> {
                    RailHint("Creating the session", "in flight")
                    PrimaryButton(text = "Starting…", onClick = {}, enabled = false)
                }
                content != null && (create is WriteState.Retryable || create is WriteState.UnknownOutcome) -> {
                    RailHint("Not started", "retry uses the same key")
                    PrimaryButton(text = "Retry start", onClick = viewModel::retryStart)
                }
                content != null && create is WriteState.Rejected -> {
                    RailHint("Rejected by the server", null)
                    PrimaryButton(text = "Start intervals", onClick = beginRun)
                    if ((create as WriteState.Rejected).status == 401) {
                        QuietButton(text = "Open Settings", onClick = onOpenSettings)
                    }
                }
                content != null && create is WriteState.Saved -> {
                    RailHint("Session created", "starting the runner")
                    PrimaryButton(text = "Start runner", onClick = container::startRunnerService)
                }
                content != null -> {
                    RailHint("${content.steps.size} steps", "download complete")
                    PrimaryButton(text = "Start intervals", onClick = beginRun)
                }
                else -> RailHint("Reading the plan", "one request, before the run")
            }
        },
    ) {
        when (val current = state) {
            is RunnerUiState.Loading -> LoadingPane()
            is RunnerUiState.NeedsConfiguration -> EmptyPane(
                eyebrow = "Configuration",
                title = "No server yet",
                detail = "Open Settings before starting an interval session.",
                action = "Open Settings",
                onAction = onOpenSettings,
            )
            is RunnerUiState.Empty -> EmptyPane(
                eyebrow = "Intervals",
                title = "No interval steps",
                detail = "This plan version has no interval session with steps for that template.",
                action = "Back to the week",
                onAction = onBack,
            )
            is RunnerUiState.Error -> ErrorPane(
                title = "The interval session did not arrive",
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
            is RunnerUiState.Content -> {
                if (snapshot != null) {
                    RunnerRunPane(snapshot = snapshot!!, report = report, completion = completion)
                } else {
                    RunnerSummary(content = current.content, create = create)
                }
            }
        }
    }
}

@Composable
private fun RunnerRail(
    snapshot: RunnerSnapshot,
    report: WriteState<dev.temper.model.CheckInEnvelope>,
    onPause: () -> Unit,
    onResume: () -> Unit,
    onDiscard: () -> Unit,
    onRetryReport: () -> Unit,
    onReport: () -> Unit,
    onDone: () -> Unit,
    onOpenSettings: () -> Unit,
) {
    when (snapshot.status) {
        RunnerStatus.Running -> {
            RailHint("Running", "step ${snapshot.stepNumber} of ${snapshot.stepCount}")
            QuietButton(text = "Pause", onClick = onPause)
        }
        RunnerStatus.Paused -> {
            RailHint("Paused", "clock stopped")
            PrimaryButton(text = "Resume", onClick = onResume)
            QuietButton(text = "End without reporting", onClick = onDiscard)
        }
        RunnerStatus.Finished -> when (report) {
            is WriteState.Sending -> {
                RailHint("Reporting the check-in", "in flight")
                PrimaryButton(text = "Reporting…", onClick = {}, enabled = false)
            }
            is WriteState.Saved -> {
                RailHint("Check-in saved", "the server answered")
                PrimaryButton(text = "Back to the week", onClick = onDone)
            }
            is WriteState.UnknownOutcome -> {
                RailHint("May be recorded", "held in memory")
                PrimaryButton(text = "Send report again", onClick = onRetryReport)
                QuietButton(text = "Back to the week", onClick = onDone)
            }
            is WriteState.Retryable -> {
                RailHint("Not sent", "retry uses the same key")
                PrimaryButton(text = "Send report again", onClick = onRetryReport)
                QuietButton(text = "Back to the week", onClick = onDone)
            }
            is WriteState.Rejected -> {
                RailHint("Rejected by the server", null)
                PrimaryButton(text = "Retry report", onClick = onReport)
                QuietButton(text = "Open Settings", onClick = onOpenSettings)
            }
            WriteState.Idle -> {
                RailHint("Not reported yet", "held in memory")
                PrimaryButton(text = "Report completion", onClick = onReport)
            }
        }
        RunnerStatus.Ready -> {
            RailHint("Ready", "waiting for the service")
        }
    }
}

@Composable
private fun RunnerSummary(content: RunnerContent, create: WriteState<SessionEnvelope>) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        AccentBlock {
            Eyebrow("Downloaded once · no network during the run")
            Text(text = content.target, color = Ink, style = MaterialTheme.typography.headlineLarge)
            Text(
                text = "${content.steps.size} steps · plan v${content.planVersionId} · ${content.date}",
                color = Ink,
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                text = if (content.existingSessionId != null) {
                    "The steps are held in memory. Starting continues the session already logged for this day, then the foreground service runs the clock until the finish."
                } else {
                    "The steps are held in memory. Starting creates the session record, then the foreground service runs the clock until the finish."
                },
                color = Ink,
                style = MaterialTheme.typography.bodyMedium,
            )
        }

        if (content.distanceRequired) {
            SyncStrip(
                mark = "!",
                title = "Distance steps need a fix",
                detail = "If the location measurement is unavailable, the repetition waits. It never advances on time alone.",
            )
        }

        CreateStrip(create)

        Column(verticalArrangement = Arrangement.spacedBy(1.dp)) {
            content.steps.forEachIndexed { index, step ->
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    MonoText(
                        text = (index + 1).toString().padStart(2, '0'),
                        color = Panel,
                        size = 13,
                        modifier = Modifier.width(34.dp),
                    )
                    Text(
                        text = step.label,
                        color = Ink,
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.weight(1f),
                    )
                    val duration = step.durationMs
                    val distance = step.distanceM
                    MonoText(
                        text = when {
                            duration != null -> formatClock(duration)
                            distance != null -> formatDistance(distance)
                            else -> "—"
                        },
                        color = Ink,
                        size = 14,
                    )
                }
                Hairline()
            }
        }

        Text(
            text = "The runner makes no request between this download and the report. Pausing stops the clock; resuming continues the same step.",
            color = Panel,
            style = MaterialTheme.typography.labelMedium,
        )
    }
}

@Composable
private fun CreateStrip(create: WriteState<SessionEnvelope>) {
    when (create) {
        is WriteState.Retryable -> SyncStrip(
            mark = "✗",
            title = "Not started",
            detail = "${create.reason} Nothing was created.",
        )
        is WriteState.UnknownOutcome -> SyncStrip(
            mark = "!",
            title = "No answer from the server",
            detail = "${create.reason} The session may exist. Retrying uses the same key.",
            dark = true,
        )
        is WriteState.Rejected -> SyncStrip(
            mark = "✗",
            title = "The server rejected the session",
            detail = "${create.message} (${create.code ?: "HTTP ${create.status}"})",
        )
        WriteState.Sending -> SyncStrip(
            mark = "…",
            title = "Creating the session",
            detail = "The first request since the plan arrived. The run starts when the server answers.",
            dashed = true,
        )
        else -> Unit
    }
}

@Composable
private fun RunnerRunPane(
    snapshot: RunnerSnapshot,
    report: WriteState<dev.temper.model.CheckInEnvelope>,
    completion: dev.temper.intervals.RunnerCompletion?,
) {
    val step = snapshot.currentStep
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        if (snapshot.finished) {
            AccentBlock {
                Eyebrow("${snapshot.stepCount} of ${snapshot.stepCount} steps")
                Text(text = "Session finished", color = Ink, style = MaterialTheme.typography.headlineLarge)
                Text(
                    text = "${formatClock(snapshot.runElapsedMs)} total · ${snapshot.stepCount} steps",
                    color = Ink,
                    style = MaterialTheme.typography.bodyLarge,
                )
                Text(
                    text = "Finishing does not report by itself. The report below uses the session created before the first step and the same key on every retry.",
                    color = Ink,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            ReportStrip(report, completion)
            Spacer(modifier = Modifier.height(4.dp))
            Text(
                text = "The report lives in memory. Closing the app before the server answers loses it.",
                color = Panel,
                style = MaterialTheme.typography.labelMedium,
            )
            return@Column
        }

        if (snapshot.status == RunnerStatus.Paused) {
            SyncStrip(
                mark = "II",
                title = "Paused",
                detail = "The clock is stopped. Resuming continues this step where it was.",
            )
        }

        if (step.isDistance && !snapshot.distanceAvailable) {
            SyncStrip(
                mark = "!",
                title = "Distance paused",
                detail = "No location for ${snapshot.distanceUnavailableForMs / 1_000L} s. This step will not advance on its own.",
                dark = true,
            )
        }

        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Eyebrow("Now · step ${snapshot.stepNumber} of ${snapshot.stepCount}")
            Text(text = step.label, color = Ink, style = MaterialTheme.typography.headlineLarge)
            val hero = when {
                step.durationMs != null -> formatClock(snapshot.stepRemainingMs ?: 0L)
                snapshot.distanceAvailable -> formatDistance(snapshot.stepDistanceRemainingM ?: 0.0)
                else -> formatDistance(snapshot.stepDistanceDoneM)
            }
            val unit = when {
                step.durationMs != null -> "left"
                snapshot.distanceAvailable -> "m to go"
                else -> "m done · last measured"
            }
            Row(verticalAlignment = Alignment.Bottom) {
                Text(
                    text = hero,
                    color = Ink,
                    fontFamily = DataFont,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 72.sp,
                    lineHeight = 76.sp,
                )
                Spacer(modifier = Modifier.width(10.dp))
                Text(
                    text = unit,
                    color = Panel,
                    style = MaterialTheme.typography.labelMedium,
                    modifier = Modifier.padding(bottom = 12.dp),
                )
            }
            val fraction = progressFraction(snapshot)
            Box(modifier = Modifier.fillMaxWidth().height(6.dp).background(PaperSoft)) {
                Box(modifier = Modifier.fillMaxWidth(fraction).height(6.dp).background(Action))
            }
        }

        Row(horizontalArrangement = Arrangement.spacedBy(20.dp)) {
            MetaBlock("Elapsed", formatClock(snapshot.stepElapsedMs))
            MetaBlock("Total", formatClock(snapshot.runElapsedMs))
            MetaBlock("Step", "${snapshot.stepNumber} / ${snapshot.stepCount}")
            if (step.isDistance) {
                MetaBlock("Rep", "${formatDistance(snapshot.stepDistanceDoneM)} / ${formatDistance(step.distanceM ?: 0.0)} m")
            }
        }

        Hairline()

        val next = snapshot.nextStep
        if (next != null) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Eyebrow("Next")
                    Text(text = next.label, color = Ink, style = MaterialTheme.typography.titleMedium)
                }
                val nextDuration = next.durationMs
                val nextDistance = next.distanceM
                MonoText(
                    text = when {
                        nextDuration != null -> formatClock(nextDuration)
                        nextDistance != null -> formatDistance(nextDistance)
                        else -> "—"
                    },
                    color = Panel,
                    size = 14,
                )
            }
        }

        Text(
            text = "Cues are local. The screen can lock and the runner keeps counting; no request is made until the report.",
            color = Panel,
            style = MaterialTheme.typography.labelMedium,
        )
    }
}

@Composable
private fun ReportStrip(
    report: WriteState<dev.temper.model.CheckInEnvelope>,
    completion: dev.temper.intervals.RunnerCompletion?,
) {
    when (report) {
        is WriteState.Saved -> SyncStrip(
            mark = "✓",
            title = "Check-in saved",
            detail = "The server answered. Evidence: ${report.value.effectiveEvidence?.source ?: "reported"} · revision ${report.value.session.revision}",
        )
        is WriteState.Sending -> SyncStrip(
            mark = "…",
            title = "Reporting the result",
            detail = "The runner has stopped. Waiting for the server.",
            dashed = true,
        )
        is WriteState.UnknownOutcome -> SyncStrip(
            mark = "!",
            title = "The report was not confirmed",
            detail = "${report.reason} The run may already be recorded. Sending again uses the same key.",
            dark = true,
        )
        is WriteState.Retryable -> SyncStrip(
            mark = "✗",
            title = "Not sent",
            detail = "${report.reason} The result is still in memory.",
        )
        is WriteState.Rejected -> SyncStrip(
            mark = "✗",
            title = "The server rejected the check-in",
            detail = "${report.message} (${report.code ?: "HTTP ${report.status}"})",
        )
        WriteState.Idle -> SyncStrip(
            mark = "…",
            title = "Not reported yet",
            detail = "The result is held in memory${completion?.let { " · ${formatClock(it.totalElapsedMs)} · ${it.stepCount} steps" } ?: ""}.",
            dashed = true,
        )
    }
}

@Composable
private fun MetaBlock(label: String, value: String) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(text = label.uppercase(Locale.US), color = Panel, style = MaterialTheme.typography.labelSmall)
        MonoText(text = value, color = Ink, size = 15)
    }
}

private fun progressFraction(snapshot: RunnerSnapshot): Float {
    val step = snapshot.currentStep
    val duration = step.durationMs
    if (duration != null && duration > 0L) {
        val remaining = snapshot.stepRemainingMs ?: 0L
        return (1.0 - remaining.toDouble() / duration.toDouble()).coerceIn(0.0, 1.0).toFloat()
    }
    val goal = step.distanceM
    if (goal != null && goal > 0.0) {
        return (snapshot.stepDistanceDoneM / goal).coerceIn(0.0, 1.0).toFloat()
    }
    return 0f
}

private fun formatClock(milliseconds: Long): String {
    val totalSeconds = (milliseconds / 1_000L).coerceAtLeast(0L)
    val hours = totalSeconds / 3_600L
    val minutes = (totalSeconds % 3_600L) / 60L
    val seconds = totalSeconds % 60L
    return if (hours > 0L) {
        String.format(Locale.US, "%d:%02d:%02d", hours, minutes, seconds)
    } else {
        String.format(Locale.US, "%d:%02d", minutes, seconds)
    }
}

private fun formatDistance(meters: Double): String =
    if (meters % 1.0 == 0.0) meters.toInt().toString() else String.format(Locale.US, "%.1f", meters)
