package dev.temper

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import dev.temper.ui.TemperNavHost
import dev.temper.ui.theme.TemperTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        val container = (application as TemperApplication).container
        setContent {
            TemperTheme {
                TemperNavHost(container)
            }
        }
    }
}
