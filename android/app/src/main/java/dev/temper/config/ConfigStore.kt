package dev.temper.config

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

data class StoredConfig(
    val endpoint: String,
    val token: String,
)

private val Context.temperDataStore by preferencesDataStore(name = "temper")

class ConfigStore(private val context: Context) {
    private val endpointKey = stringPreferencesKey("endpoint")
    private val tokenCiphertextKey = stringPreferencesKey("token_ciphertext")
    private val tokenKeyAlias = stringPreferencesKey("token_key_alias")

    suspend fun read(): StoredConfig? {
        val preferences = runCatching { context.temperDataStore.data.first() }.getOrNull() ?: return null
        val endpoint = preferences[endpointKey]?.takeIf { it.isNotBlank() } ?: return null
        val ciphertext = preferences[tokenCiphertextKey]?.takeIf { it.isNotBlank() } ?: return null
        val token = runCatching { TokenCipher.decrypt(ciphertext) }.getOrNull()
        if (token.isNullOrBlank()) {
            runCatching {
                context.temperDataStore.edit { values -> values.remove(tokenCiphertextKey) }
            }
            return null
        }
        return StoredConfig(endpoint, token)
    }

    suspend fun save(endpoint: String, token: String) {
        val ciphertext = TokenCipher.encrypt(token)
        context.temperDataStore.edit { values ->
            values[endpointKey] = endpoint
            values[tokenCiphertextKey] = ciphertext
            values[tokenKeyAlias] = TokenCipher.KEY_ALIAS
        }
    }

    suspend fun clear() {
        context.temperDataStore.edit { values -> values.clear() }
        TokenCipher.delete()
    }
}
