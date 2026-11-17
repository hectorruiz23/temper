package dev.temper.ui.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.collectAsState
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel
import dev.temper.AppContainer
import dev.temper.ui.components.AccentBlock
import dev.temper.ui.components.Eyebrow
import dev.temper.ui.components.Hairline
import dev.temper.ui.components.PrimaryButton
import dev.temper.ui.components.QuietButton
import dev.temper.ui.components.RailHint
import dev.temper.ui.components.SmallTextButton
import dev.temper.ui.components.SyncStrip
import dev.temper.ui.components.TemperScaffold
import dev.temper.ui.components.TemperTextField
import dev.temper.ui.components.TemperTopBar
import dev.temper.ui.health.HealthSyncBlock
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel
import dev.temper.ui.TemperSimpleFactory

@Composable
fun SettingsScreen(
    container: AppContainer,
    canGoBack: Boolean,
    onConfigured: () -> Unit,
    onBack: () -> Unit,
) {
    val viewModel = viewModel<SettingsViewModel>(factory = TemperSimpleFactory { SettingsViewModel(container) })
    val state by viewModel.state.collectAsState()

    LaunchedEffect(state.save) {
        if (state.save is SaveState.Saved) onConfigured()
    }

    TemperScaffold(
        topBar = {
            TemperTopBar(
                title = "Settings",
                subtitle = "Server and token",
                onBack = if (canGoBack) onBack else null,
            )
        },
        rail = {
            RailHint(
                left = if (state.hasStoredToken) "Stored on this phone" else "Not configured yet",
                right = if (state.save is SaveState.Saving) "saving" else null,
            )
            PrimaryButton(
                text = when (state.save) {
                    is SaveState.Saving -> "Saving…"
                    else -> "Save and continue"
                },
                onClick = viewModel::save,
                enabled = state.save !is SaveState.Saving,
            )
            QuietButton(
                text = if (state.connection is ConnectionState.Testing) "Testing…" else "Test connection",
                onClick = viewModel::testConnection,
                enabled = state.save !is SaveState.Saving,
            )
        },
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            AccentBlock {
                Eyebrow("Temper · Android")
                Text("Server and token", style = androidx.compose.material3.MaterialTheme.typography.headlineLarge, color = Ink)
                Text(
                    "The endpoint must be HTTPS. The token is stored encrypted with this phone's Keystore and never leaves the device except in the X-Temper-Token header.",
                    style = androidx.compose.material3.MaterialTheme.typography.bodyMedium,
                    color = Ink,
                )
            }

            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Eyebrow("Endpoint")
                TemperTextField(
                    value = state.endpoint,
                    onValueChange = viewModel::onEndpointChange,
                    label = "https://host[:port]",
                    enabled = state.save !is SaveState.Saving,
                    isError = state.endpointError != null,
                    supportingText = state.endpointError ?: "The server root, without a path.",
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                )
                Hairline()
                Eyebrow("Service token")
                TemperTextField(
                    value = state.token,
                    onValueChange = viewModel::onTokenChange,
                    label = "X-Temper-Token",
                    enabled = state.save !is SaveState.Saving,
                    isError = state.tokenError != null,
                    supportingText = state.tokenError
                        ?: if (state.hasStoredToken) "A token is already stored. Leave this blank to keep it." else "Paste the token from the server's environment file.",
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                )
            }

            when (val connection = state.connection) {
                is ConnectionState.Testing -> SyncStrip(
                    mark = "…",
                    title = "Testing the connection",
                    detail = "Asking GET /api/health.",
                    dashed = true,
                )
                is ConnectionState.Passed -> SyncStrip(
                    mark = "✓",
                    title = "Server reachable",
                    detail = connection.message,
                )
                is ConnectionState.Failed -> SyncStrip(
                    mark = "✗",
                    title = "Connection failed",
                    detail = connection.message,
                )
                ConnectionState.Idle -> Unit
            }

            when (val save = state.save) {
                is SaveState.Failed -> SyncStrip(
                    mark = "✗",
                    title = "Not stored",
                    detail = save.message,
                )
                SaveState.Saved -> SyncStrip(
                    mark = "✓",
                    title = "Configuration stored",
                    detail = "The endpoint and the encrypted token live on this phone only.",
                )
                else -> Unit
            }

            Text(
                text = "The health check proves the address, DNS, TLS and the reverse proxy. The token is proven by the first authenticated read on the week screen.",
                style = androidx.compose.material3.MaterialTheme.typography.labelMedium,
                color = Panel,
            )

            HealthSyncBlock(container)

            if (state.hasStoredToken) {
                Spacer(modifier = Modifier.height(8.dp))
                Hairline()
                SmallTextButton(
                    text = "Forget configuration",
                    onClick = viewModel::forget,
                    modifier = Modifier.fillMaxWidth(),
                )
            }
        }
    }
}
