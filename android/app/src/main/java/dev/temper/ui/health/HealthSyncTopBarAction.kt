package dev.temper.ui.health

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.res.painterResource
import androidx.health.connect.client.PermissionController
import dev.temper.AppContainer
import dev.temper.R
import dev.temper.health.HealthPhase
import dev.temper.ui.components.TopBarIconAction

@Composable
fun HealthSyncTopBarAction(container: AppContainer) {
    val state by container.healthSync.state.collectAsState()
    val launcher = rememberLauncherForActivityResult(
        PermissionController.createRequestPermissionResultContract(),
    ) { granted ->
        container.healthSync.onPermissionsResult(granted)
    }

    val busy = state.phase is HealthPhase.Checking ||
        state.phase is HealthPhase.Syncing ||
        state.phase is HealthPhase.RequestingPermissions

    TopBarIconAction(
        painter = painterResource(R.drawable.ic_sync),
        contentDescription = "Sync health data",
        enabled = !busy,
        onClick = {
            container.healthSync.syncNow { permissions -> launcher.launch(permissions) }
        },
    )
}
