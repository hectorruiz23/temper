package dev.temper

import dev.temper.config.EndpointValidation
import dev.temper.config.normalizeEndpoint
import dev.temper.config.validateConfiguration
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ConfigValidationTest {
    @Test
    fun endpointIsNormalizedAndHttpsIsRequired() {
        val valid = normalizeEndpoint("HTTPS://Temper.Example.dev/")
        assertEquals(EndpointValidation.Valid("https://temper.example.dev"), valid)
        assertTrue(normalizeEndpoint("http://temper.example.dev") is EndpointValidation.Invalid)
        assertTrue(normalizeEndpoint("temper.example.dev") is EndpointValidation.Invalid)
        assertTrue(normalizeEndpoint("https://temper.example.dev/api") is EndpointValidation.Invalid)
    }

    @Test
    fun storedTokenIsKeptWhenTheFieldIsBlank() {
        val validation = validateConfiguration("https://temper.example.dev", "", "stored-token")
        assertTrue(validation.valid)
        assertEquals("https://temper.example.dev", validation.endpoint)
        assertEquals("stored-token", validation.token)
    }

    @Test
    fun whitespaceInTokenIsRejected() {
        val validation = validateConfiguration("https://temper.example.dev", "bad token")
        assertFalse(validation.valid)
    }
}
