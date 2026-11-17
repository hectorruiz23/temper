package dev.temper.health

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.temper.ui.components.Eyebrow
import dev.temper.ui.components.Hairline
import dev.temper.ui.components.PrimaryButton
import dev.temper.ui.components.TemperScaffold
import dev.temper.ui.components.TemperTopBar
import dev.temper.ui.theme.Ink
import dev.temper.ui.theme.Panel
import dev.temper.ui.theme.TemperTheme

class PermissionsRationaleActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            TemperTheme {
                RationaleScreen(onClose = { finish() })
            }
        }
    }
}

@Composable
private fun RationaleScreen(onClose: () -> Unit) {
    TemperScaffold(
        topBar = { TemperTopBar(title = "Health Connect", subtitle = "Read only") },
        rail = {
            PrimaryButton(text = "Back", onClick = onClose)
        },
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Eyebrow("What Temper reads")
            Text(
                text = "Temper reads sleep sessions, step records, resting heart rate, heart rate variability and exercise sessions from Health Connect.",
                color = Ink,
                style = MaterialTheme.typography.bodyLarge,
            )
            Hairline()
            Eyebrow("Why")
            Text(
                text = "The owner uses those records to keep one daily row of training context on their own server. A day is aggregated on the phone and is readable only when the owner signs in with the service token.",
                color = Ink,
                style = MaterialTheme.typography.bodyLarge,
            )
            Hairline()
            Eyebrow("What leaves the phone")
            Text(
                text = "Only the aggregated day row is sent to the owner's Temper server. Raw Health Connect records, the watch's history and this permission screen never leave the phone.",
                color = Ink,
                style = MaterialTheme.typography.bodyLarge,
            )
            Hairline()
            Eyebrow("What Temper never does")
            Text(
                text = "Temper never writes to Health Connect, never reads in the background and never schedules a health sync. Reads happen only when the owner syncs or opens the app.",
                color = Panel,
                style = MaterialTheme.typography.bodyMedium,
            )
        }
    }
}
