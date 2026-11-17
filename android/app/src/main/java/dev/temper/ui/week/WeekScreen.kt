package dev.temper.ui.week

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.model.Evidence
import dev.temper.model.PerformedSession
import dev.temper.model.PlannedSession
import dev.temper.model.WeekResponse
import dev.temper.ui.TemperSimpleFactory
import dev.temper.ui.components.AccentBlock
import dev.temper.ui.components.EmptyPane
import dev.temper.ui.components.ErrorPane
import dev.temper.ui.components.Eyebrow
import dev.temper.ui.components.Hairline
import dev.temper.ui.components.LoadingPane
import dev.temper.ui.components.RailHint
import dev.temper.ui.components.TemperScaffold
import dev.temper.ui.components.TemperTopBar
import dev.temper.ui.components.TopBarAction
import dev.temper.ui.parseDate
import dev.temper.ui.theme.Card
import dev.temper.ui.theme.DataFont
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel
import dev.temper.ui.weekLabel
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import java.util.Locale

@Composable
fun WeekScreen(
    container: AppContainer,
    onOpenSession: (String, String) -> Unit,
    onOpenIntervals: (String, String, Long) -> Unit,
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
                    val first = todayPlanned.firstOrNull()
                    if (first != null) {
                        RailHint("Today · ${first.target.ifBlank { first.discipline }}", "plan v${first.planVersionId}")
                    } else {
                        RailHint("Today · rest", "nothing planned")
                    }
                }
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
                onOpenIntervals = onOpenIntervals,
            )
        }
    }
}

@Composable
private fun WeekContent(
    week: WeekResponse,
    today: LocalDate,
    onOpenSession: (String, String) -> Unit,
    onOpenIntervals: (String, String, Long) -> Unit,
) {
    val start = parseDate(week.weekStart) ?: today
    val days = (0..6).map { start.plusDays(it.toLong()) }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 12.dp, bottom = 16.dp),
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
            val performed = week.performed.firstOrNull { it.date == date.toString() }
            if (date == today) {
                TodayRow(
                    date = date,
                    planned = planned,
                    performed = performed,
                    onOpenSession = onOpenSession,
                    onOpenIntervals = onOpenIntervals,
                )
            } else {
                DayRow(
                    date = date,
                    planned = planned,
                    performed = performed,
                    onOpenSession = onOpenSession,
                    onOpenIntervals = onOpenIntervals,
                )
            }
        }
    }
}

@Composable
private fun DayRow(
    date: LocalDate,
    planned: List<PlannedSession>,
    performed: PerformedSession?,
    onOpenSession: (String, String) -> Unit,
    onOpenIntervals: (String, String, Long) -> Unit,
) {
    val session = planned.firstOrNull()
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .height(56.dp)
            .background(Card.copy(alpha = 0.0f))
            .clickable(enabled = session != null) {
                if (session != null) open(session, date, onOpenSession, onOpenIntervals)
            },
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
            val summary = buildList {
                if (planned.isNotEmpty()) {
                    add(planned.joinToString(" · ") { it.discipline } + " · plan v${planned.first().planVersionId}")
                }
                performedLine(performed)?.let(::add)
            }.joinToString(" · ")
            if (summary.isNotEmpty()) {
                Text(
                    text = summary,
                    color = Panel,
                    style = MaterialTheme.typography.labelMedium,
                    maxLines = 1,
                )
            }
        }
        Text(
            text = when {
                performed?.complete == true -> "✓"
                planned.isEmpty() && performed == null -> "—"
                else -> ""
            },
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
    performed: PerformedSession?,
    onOpenSession: (String, String) -> Unit,
    onOpenIntervals: (String, String, Long) -> Unit,
) {
    val session = planned.firstOrNull()
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp)
            .clickable(enabled = session != null) {
                if (session != null) open(session, date, onOpenSession, onOpenIntervals)
            },
    ) {
        AccentBlock {
            Eyebrow("Today · ${planned.firstOrNull()?.discipline ?: "rest"}")
            Text(
                text = planned.firstOrNull()?.target?.ifBlank { "Rest day" } ?: "Rest day",
                color = Ink,
                style = MaterialTheme.typography.headlineLarge,
            )
            val summary = if (planned.isEmpty()) {
                "Nothing planned. The server answered; there is no session for ${date.dayOfMonth}."
            } else {
                "${planned.size} planned · plan v${planned.first().planVersionId}"
            }
            Text(
                text = performedLine(performed)?.let { "$summary · $it" } ?: summary,
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
        }
    }
    Hairline()
}

private fun open(
    session: PlannedSession,
    date: LocalDate,
    onOpenSession: (String, String) -> Unit,
    onOpenIntervals: (String, String, Long) -> Unit,
) {
    val sessionDate = session.date.ifBlank { date.toString() }
    if (session.discipline.equals("intervals", ignoreCase = true)) {
        onOpenIntervals(sessionDate, session.templateId, session.planVersionId)
    } else {
        onOpenSession(sessionDate, session.templateId)
    }
}

private fun performedLine(performed: PerformedSession?): String? {
    if (performed == null) return null
    val marker = if (performed.complete) "Logged · complete" else "Logged · open"
    val evidence = evidenceLine(performed.evidence)
    return if (evidence == null) marker else "$marker · $evidence"
}

private fun evidenceLine(evidence: Evidence?): String? {
    if (evidence == null) return null
    val source = evidence.source.takeIf { it.isNotBlank() }
    val detail = evidence.reportedCompletedAt?.takeIf { it.isNotBlank() }
        ?: evidence.workoutId?.takeIf { it.isNotBlank() }
    return when {
        source != null && detail != null -> "$source $detail"
        source != null -> source
        detail != null -> detail
        else -> null
    }
}
