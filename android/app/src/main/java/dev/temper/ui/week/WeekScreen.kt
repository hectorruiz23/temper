package dev.temper.ui.week

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
import androidx.compose.foundation.layout.size
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.model.PlannedSession
import dev.temper.model.WeekDay
import dev.temper.model.WeekResponse
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
import dev.temper.ui.components.TopBarAction
import dev.temper.ui.dayLabel
import dev.temper.ui.formatSleep
import dev.temper.ui.parseDate
import dev.temper.ui.theme.Card
import dev.temper.ui.theme.DataFont
import dev.temper.ui.theme.Hair
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel
import dev.temper.ui.theme.PaperSoft
import dev.temper.ui.weekLabel
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.util.Locale

@Composable
fun WeekScreen(
    container: AppContainer,
    onOpenSession: (String, String) -> Unit,
    onOpenRunner: (String, String, Long) -> Unit,
    onOpenSettings: () -> Unit,
) {
    val viewModel = viewModel<WeekViewModel>(factory = TemperSimpleFactory { WeekViewModel(container) })
    val state by viewModel.state.collectAsState()

    LaunchedEffect(Unit) {
        viewModel.load()
    }

    val content = state as? WeekUiState.Content
    val today = content?.today ?: LocalDate.now()
    val todayPlanned = content?.week?.planned?.filter { it.date == today.toString() }.orEmpty()
    val firstStrength = todayPlanned.firstOrNull { it.discipline == "strength" }
    val firstIntervals = todayPlanned.firstOrNull { it.discipline == "intervals" }

    TemperScaffold(
        topBar = {
            TemperTopBar(
                title = "This week",
                subtitle = content?.let { weekLabel(it.week.weekStart, it.week.weekEnd) } ?: "Reading the server",
                trailing = { TopBarAction("Settings", onOpenSettings) },
            )
        },
        rail = {
            when (state) {
                is WeekUiState.Loading -> RailHint("Waiting for the server", "nothing cached")
                is WeekUiState.NeedsConfiguration -> RailHint("No endpoint or token", "open Settings")
                is WeekUiState.Error -> RailHint("No answer", "07:42")
                is WeekUiState.Content -> {
                    if (firstStrength != null) {
                        RailHint("Today · ${firstStrength.target.ifBlank { "strength" }}", "plan v${firstStrength.planVersionId}")
                    } else if (firstIntervals != null) {
                        RailHint("Today · ${firstIntervals.target.ifBlank { "intervals" }}", "plan v${firstIntervals.planVersionId}")
                    } else if (todayPlanned.isNotEmpty()) {
                        RailHint("Today · ${todayPlanned.first().discipline}", "plan v${todayPlanned.first().planVersionId}")
                    } else {
                        RailHint("Today · rest", "nothing planned")
                    }
                }
            }
            when {
                state is WeekUiState.Content && firstStrength != null -> PrimaryButton(
                    text = "Start session",
                    onClick = { onOpenSession(today.toString(), firstStrength.templateId) },
                )
                state is WeekUiState.Content && firstIntervals != null -> PrimaryButton(
                    text = "Start intervals",
                    onClick = {
                        onOpenRunner(today.toString(), firstIntervals.templateId, firstIntervals.planVersionId)
                    },
                )
                state is WeekUiState.Content -> PrimaryButton(text = "Check again", onClick = viewModel::load)
                state is WeekUiState.NeedsConfiguration -> PrimaryButton(text = "Open Settings", onClick = onOpenSettings)
                state is WeekUiState.Error -> PrimaryButton(text = "Try again", onClick = viewModel::load)
                else -> Unit
            }
        },
    ) {
        when (val current = state) {
            is WeekUiState.Loading -> LoadingPane()
            is WeekUiState.NeedsConfiguration -> EmptyPane(
                eyebrow = "Configuration",
                title = "No server yet",
                detail = "This phone has no endpoint or token. Open Settings to point it at your Temper server.",
                action = "Open Settings",
                onAction = onOpenSettings,
            )
            is WeekUiState.Error -> ErrorPane(
                title = "The server did not answer",
                detail = current.message,
                action = if (current.retryable) "Try again" else "Open Settings",
                onAction = if (current.retryable) viewModel::load else onOpenSettings,
            )
            is WeekUiState.Content -> WeekContent(
                week = current.week,
                today = current.today,
                onOpenSession = onOpenSession,
                onOpenRunner = onOpenRunner,
            )
        }
    }
}

@Composable
private fun WeekContent(
    week: WeekResponse,
    today: LocalDate,
    onOpenSession: (String, String) -> Unit,
    onOpenRunner: (String, String, Long) -> Unit,
) {
    val start = parseDate(week.weekStart) ?: today
    val days = (0..6).map { start.plusDays(it.toLong()) }
    val healthToday = week.days.firstOrNull { it.date == today.toString() }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 16.dp),
    ) {
        if (week.planned.isEmpty()) {
            item {
                EmptyPane(
                    eyebrow = "Week ${today.get(java.time.temporal.WeekFields.ISO.weekOfWeekBasedYear())} · no plan",
                    title = "No plan for this week",
                    detail = "The server answered and there is no plan version in effect for these dates. Plans are written with Archi and stored on the Pi.",
                )
            }
        }
        items(days) { date ->
            val planned = week.planned.filter { it.date == date.toString() }
            if (date == today) {
                TodayRow(
                    date = date,
                    planned = planned,
                    performed = week.performed.any { it.date == date.toString() && it.complete },
                    onOpenSession = onOpenSession,
                    onOpenRunner = onOpenRunner,
                )
            } else {
                DayRow(date = date, planned = planned, performed = week.performed.any { it.date == date.toString() && it.complete })
            }
        }
        item {
            Spacer(modifier = Modifier.height(8.dp))
            HealthBlock(day = healthToday, today = today)
        }
    }
}

@Composable
private fun DayRow(date: LocalDate, planned: List<PlannedSession>, performed: Boolean) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(56.dp)
            .background(Card.copy(alpha = 0.0f)),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.width(54.dp)) {
            Text(
                text = date.format(DateTimeFormatter.ofPattern("EEE", Locale.ENGLISH)).uppercase(),
                color = Panel,
                style = MaterialTheme.typography.labelSmall,
            )
            Text(
                text = date.dayOfMonth.toString(),
                color = Ink,
                style = MaterialTheme.typography.titleLarge,
            )
        }
        Column(modifier = Modifier.weight(1f)) {
            val first = planned.firstOrNull()
            Text(
                text = first?.target?.takeIf { target -> target.isNotBlank() } ?: first?.discipline ?: "Rest",
                color = if (planned.isEmpty()) Panel else Ink,
                style = MaterialTheme.typography.titleMedium,
                fontWeight = if (planned.isEmpty()) FontWeight.Normal else FontWeight.SemiBold,
                maxLines = 1,
            )
            if (planned.isNotEmpty()) {
                Text(
                    text = planned.joinToString(" · ") { it.discipline } + " · plan v${planned.first().planVersionId}",
                    color = Panel,
                    style = MaterialTheme.typography.labelMedium,
                    maxLines = 1,
                )
            }
        }
        Text(
            text = if (performed) "✓" else if (planned.isEmpty()) "—" else "",
            color = Ink,
            fontFamily = DataFont,
            style = MaterialTheme.typography.titleMedium,
        )
    }
    Hairline()
}

@Composable
private fun TodayRow(
    date: LocalDate,
    planned: List<PlannedSession>,
    performed: Boolean,
    onOpenSession: (String, String) -> Unit,
    onOpenRunner: (String, String, Long) -> Unit,
) {
    Column(modifier = Modifier.padding(vertical = 6.dp)) {
        AccentBlock {
            Eyebrow("Today · ${planned.firstOrNull()?.discipline ?: "rest"}")
            Text(
                text = planned.firstOrNull()?.target?.ifBlank { "Rest day" } ?: "Rest day",
                color = Ink,
                style = MaterialTheme.typography.headlineLarge,
            )
            Text(
                text = if (planned.isEmpty()) {
                    "Nothing planned. The server answered; there is no session for ${date.dayOfMonth}."
                } else {
                    "${planned.size} planned · plan v${planned.first().planVersionId}" + if (performed) " · closed" else ""
                },
                color = Ink,
                style = MaterialTheme.typography.bodyMedium,
            )
            if (planned.size > 1) {
                planned.drop(1).forEach { entry ->
                    Text(
                        text = "${entry.target.ifBlank { entry.discipline }} · ${entry.discipline}",
                        color = Panel,
                        style = MaterialTheme.typography.labelMedium,
                    )
                }
            }
            val strength = planned.firstOrNull { it.discipline == "strength" }
            if (strength != null) {
                Spacer(modifier = Modifier.height(4.dp))
                QuietButton(
                    text = "Open session",
                    onClick = { onOpenSession(date.toString(), strength.templateId) },
                )
            }
            val intervals = planned.firstOrNull { it.discipline == "intervals" }
            if (intervals != null) {
                Spacer(modifier = Modifier.height(4.dp))
                QuietButton(
                    text = "Start intervals",
                    onClick = { onOpenRunner(date.toString(), intervals.templateId, intervals.planVersionId) },
                )
            }
        }
    }
    Hairline()
}

@Composable
private fun HealthBlock(day: WeekDay?, today: LocalDate) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(PaperSoft)
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Eyebrow("Day data · ${today.format(DateTimeFormatter.ofPattern("d MMM", Locale.ENGLISH))}")
        if (day == null) {
            Text(
                text = "No health data for today. Unavailable values are not shown as zero.",
                color = Panel,
                style = MaterialTheme.typography.labelMedium,
            )
            return@Column
        }
        Text(
            text = "Sleep ${formatSleep(day.sleepMs)} · Steps ${day.steps ?: "—"}",
            color = Ink,
            style = MaterialTheme.typography.bodyLarge,
        )
        Text(
            text = "Resting HR ${day.restingHrBpm ?: "—"} · HRV ${day.hrvRmssdMs?.let { it.toString() } ?: "—"} ms",
            color = Ink,
            style = MaterialTheme.typography.bodyLarge,
        )
    }
}
