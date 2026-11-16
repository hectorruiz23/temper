package dev.temper.config

import java.net.URI

sealed interface EndpointValidation {
    data class Valid(val endpoint: String) : EndpointValidation
    data class Invalid(val message: String) : EndpointValidation
}

data class ConfigurationValidation(
    val endpoint: String?,
    val token: String?,
    val endpointError: String? = null,
    val tokenError: String? = null,
) {
    val valid: Boolean get() = endpoint != null && token != null
}

fun normalizeEndpoint(raw: String): EndpointValidation {
    val value = raw.trim()
    if (value.isEmpty()) return EndpointValidation.Invalid("Enter the server address.")
    if (value.any { it.isWhitespace() }) return EndpointValidation.Invalid("The address cannot contain spaces.")
    val uri = runCatching { URI(value) }.getOrNull()
        ?: return EndpointValidation.Invalid("That is not a valid address.")
    if (!uri.scheme.equals("https", ignoreCase = true)) {
        return EndpointValidation.Invalid("The endpoint must start with https://.")
    }
    if (uri.host.isNullOrBlank()) return EndpointValidation.Invalid("The address needs a host.")
    if (uri.userInfo != null) return EndpointValidation.Invalid("Credentials cannot be part of the address.")
    val path = uri.rawPath
    if (!path.isNullOrEmpty() && path != "/") {
        return EndpointValidation.Invalid("Use the server root, without a path.")
    }
    if (uri.rawQuery != null || uri.rawFragment != null) {
        return EndpointValidation.Invalid("Use the server root, without a query or fragment.")
    }
    if (uri.port != -1 && (uri.port < 1 || uri.port > 65535)) {
        return EndpointValidation.Invalid("The port must be between 1 and 65535.")
    }
    val host = uri.host.lowercase()
    val port = if (uri.port == -1) "" else ":${uri.port}"
    return EndpointValidation.Valid("https://$host$port")
}

fun validateConfiguration(rawEndpoint: String, rawToken: String, storedToken: String? = null): ConfigurationValidation {
    val endpointValidation = normalizeEndpoint(rawEndpoint)
    val typedToken = rawToken.trim()
    val token = when {
        typedToken.isNotEmpty() -> typedToken
        !storedToken.isNullOrBlank() -> storedToken
        else -> null
    }
    val endpoint = (endpointValidation as? EndpointValidation.Valid)?.endpoint
    val endpointError = (endpointValidation as? EndpointValidation.Invalid)?.message
    val tokenError = when {
        token == null -> "Enter the service token."
        token.any { it.isWhitespace() } -> "The token cannot contain spaces."
        else -> null
    }
    return ConfigurationValidation(
        endpoint = endpoint,
        token = if (tokenError == null) token else null,
        endpointError = endpointError,
        tokenError = tokenError,
    )
}
