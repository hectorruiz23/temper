package dev.temper.api

import kotlinx.serialization.json.Json

val TemperJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = false
}
