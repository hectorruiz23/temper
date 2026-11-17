package dev.temper.ui.health

import android.content.Context
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.health.connect.client.HealthConnectClient
import androidx.health.connect.client.PermissionController
import dev.temper.AppContainer
import dev.temper.health.HealthAvailability
import dev.temper.health.HealthDaySnapshot
import dev.temper.health.HealthPermission
import dev.temper.health.HealthPermissionState
import dev.temper.health.HealthPhase
import dev.temper.health.HealthUiState
import dev.temper.ui.components.Eyebrow
import dev.temper.ui.components.SmallTextButton
import dev.temper.ui.components.SyncStrip
import dev.temper.ui.formatSleep
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel
import dev.temper.ui.theme.PaperSoft
import java.math.BigDecimal
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

@Composable
fun HealthSyncBlock(container: AppContainer) {
    val state by container.healthSync.state.collectAsState()
    val context = LocalContext.current
    val launcher = rememberLauncherForActivityResult(
        PermissionController.createRequestPermissionResultContract(),
    ) { granted ->
        container.healthSync.onPermissionsResult(granted)
    }

    HealthSyncContent(
        state = state,
        onSync = {
            container.healthSync.syncNow { permissions ->
                launcher.launch(permissions)
            }
        },
        onOpenHealthConnect = { openHealthConnect(context) },
    )
}

@Composable
private fun HealthSyncContent(
    state: HealthUiState,
    onSync: () -> Unit,
    onOpenHealthConnect: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(PaperSoft)
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Eyebrow("Health Connect · read only")

        when (val phase = state.phase) {
            is HealthPhase.Unavailable -> SyncStrip(
                mark = "✗",
                title = if (phase.availability == HealthAvailability.UpdateRequired) {
                    "Health Connect needs an update"
                } else {
                    "Health Connect is not installed"
                },
                detail = if (phase.availability == HealthAvailability.UpdateRequired) {
                    "Install the Health Connect update from the Play Store, then sync again."
                } else {
                    "This phone has no Health Connect provider. The rest of Temper still works."
                },
            )
            is HealthPhase.PermissionsDenied -> SyncStrip(
                mark = "✗",
                title = if (phase.revoked) "Read permissions were revoked" else "Read permissions were denied",
                detail = missingDetail(phase.missing) +
                    if (phase.revoked) {
                        " They were granted before. Open Health Connect and grant them again."
                    } else {
                        " Nothing is read from Health Connect until they are granted."
                    },
            )
            is HealthPhase.Failed -> SyncStrip(
                mark = "✗",
                title = "Health sync failed",
                detail = phase.message,
            )
            is HealthPhase.NoData -> SyncStrip(
                mark = "·",
                title = "No health data for ${phase.date}",
                detail = "Health Connect is available and the permissions are granted, but no records exist for today. Nothing was uploaded.",
            )
            is HealthPhase.Uploaded -> SyncStrip(
                mark = "✓",
                title = if (phase.unchanged) "The day is already current" else "Day uploaded",
                detail = "Revision ${phase.revision} · ${shortTime(state.lastSyncAt?.toEpochMilli(), state.zone)}",
            )
            HealthPhase.Checking -> SyncStrip(
                mark = "…",
                title = "Checking Health Connect",
                detail = "Reading the permission state.",
                dashed = true,
            )
            HealthPhase.Syncing -> SyncStrip(
                mark = "…",
                title = "Uploading today's snapshot",
                detail = "Reading the source records and posting the day.",
                dashed = true,
            )
            is HealthPhase.RequestingPermissions -> SyncStrip(
                mark = "…",
                title = "Waiting for Health Connect permissions",
                detail = missingDetail(phase.missing),
                dashed = true,
            )
            HealthPhase.Idle -> Text(
                text = "Reads today's records when you sync or return to the app. Nothing is scheduled and no health data is stored on this phone.",
                color = Ink,
                style = MaterialTheme.typography.labelMedium,
            )
        }

        PermissionRows(state)

        val snapshot = state.snapshot
        if (snapshot != null) {
            FoundSummary(snapshot)
        }

        if (state.lastSyncAt != null) {
            Text(
                text = "Last sync ${shortTime(state.lastSyncAt.toEpochMilli(), state.zone)} · ${state.zone.id}",
                color = Panel,
                style = MaterialTheme.typography.labelMedium,
            )
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            SmallTextButton(
                text = "Sync now",
                onClick = onSync,
                modifier = Modifier.weight(1f),
            )
            if (state.permission != HealthPermissionState.Granted) {
                SmallTextButton(
                    text = "Open Health Connect",
                    onClick = onOpenHealthConnect,
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}

@Composable
private fun PermissionRows(state: HealthUiState) {
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        HealthPermission.all.chunked(2).forEach { pair ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                pair.forEach { permission ->
                    val mark = when {
                        state.granted.contains(permission) -> "✓"
                        state.missing.contains(permission) -> "✗ off"
                        else -> "·"
                    }
                    Text(
                        text = "${permission.label} $mark",
                        color = Ink,
                        style = MaterialTheme.typography.labelMedium,
                        modifier = Modifier.weight(1f),
                    )
                }
                if (pair.size == 1) Spacer(modifier = Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun FoundSummary(snapshot: HealthDaySnapshot) {
    Text(
        text = "Sleep ${formatSleep(snapshot.sleepMs)} · Steps ${snapshot.steps ?: "—"}",
        color = Ink,
        style = MaterialTheme.typography.bodyMedium,
    )
    Text(
        text = "Resting HR ${snapshot.restingHrBpm ?: "—"} · HRV ${formatDecimal(snapshot.hrvRmssdMs)} ms",
        color = Ink,
        style = MaterialTheme.typography.bodyMedium,
    )
    Text(
        text = if (snapshot.workouts.isEmpty()) {
            "No workouts in Health Connect for this day"
        } else {
            "${snapshot.workouts.size} workout${if (snapshot.workouts.size == 1) "" else "s"} · ${snapshot.workouts.joinToString(", ") { it.discipline }}"
        },
        color = Panel,
        style = MaterialTheme.typography.labelMedium,
    )
}

private fun missingDetail(missing: List<HealthPermission>): String =
    if (missing.isEmpty()) {
        "Health Connect did not grant the read permissions."
    } else {
        "${missing.joinToString(", ") { it.label }} will not be sent. Everything else keeps working."
    }

private fun formatDecimal(value: Double?): String {
    if (value == null) return "—"
    return BigDecimal.valueOf(value).stripTrailingZeros().toPlainString()
}

private fun shortTime(epochMillis: Long?, zone: ZoneId): String {
    if (epochMillis == null) return "—"
    return DateTimeFormatter.ofPattern("HH:mm · d MMM", Locale.ENGLISH)
        .withZone(zone)
        .format(java.time.Instant.ofEpochMilli(epochMillis))
}

private fun openHealthConnect(context: Context) {
    runCatching {
        context.startActivity(HealthConnectClient.getHealthConnectManageDataIntent(context))
    }
}
