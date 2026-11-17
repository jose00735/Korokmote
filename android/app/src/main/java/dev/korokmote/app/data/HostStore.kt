package dev.korokmote.app.data

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.Serializable
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/** A paired PC. The device token is stored encrypted with a Keystore key. */
@Serializable
data class Host(
    val id: String,
    val name: String,
    val address: String,
    val port: Int,
    val fingerprint: String,
    val mac: String? = null,
    val tokenEnc: String,
    val lastConnected: Long = 0,
)

class HostStore(context: Context) {
    private val prefs = context.getSharedPreferences("hosts", Context.MODE_PRIVATE)
    private val json = Json { ignoreUnknownKeys = true }
    private val _hosts = MutableStateFlow(load())
    val hosts: StateFlow<List<Host>> = _hosts

    /** Stable per-install device id sent to daemons. */
    val deviceId: String = prefs.getString("device_id", null) ?: UUID.randomUUID().toString().also {
        prefs.edit().putString("device_id", it).apply()
    }

    var lastHostId: String?
        get() = prefs.getString("last_host", null)
        set(v) = prefs.edit().putString("last_host", v).apply()

    private fun load(): List<Host> = runCatching {
        json.decodeFromString(ListSerializer(Host.serializer()), prefs.getString("list", "[]")!!)
    }.getOrDefault(emptyList())

    private fun save(list: List<Host>) {
        prefs.edit().putString("list", json.encodeToString(ListSerializer(Host.serializer()), list)).apply()
        _hosts.value = list
    }

    fun get(id: String): Host? = _hosts.value.firstOrNull { it.id == id }

    fun upsert(h: Host) = save(_hosts.value.filterNot { it.id == h.id } + h)

    fun remove(id: String) {
        save(_hosts.value.filterNot { it.id == id })
        if (lastHostId == id) lastHostId = null
    }

    fun update(id: String, f: (Host) -> Host) {
        get(id)?.let { upsert(f(it)) }
    }

    fun token(h: Host): String? = runCatching { decrypt(h.tokenEnc) }.getOrNull()

    fun encryptToken(token: String): String = encrypt(token)

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return gen.generateKey()
    }

    private fun encrypt(plain: String): String {
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.ENCRYPT_MODE, key())
        val out = c.iv + c.doFinal(plain.toByteArray())
        return Base64.encodeToString(out, Base64.NO_WRAP)
    }

    private fun decrypt(enc: String): String {
        val raw = Base64.decode(enc, Base64.NO_WRAP)
        val c = Cipher.getInstance("AES/GCM/NoPadding")
        c.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, raw, 0, 12))
        return String(c.doFinal(raw, 12, raw.size - 12))
    }

    private companion object {
        const val ALIAS = "korokmote_tokens"
    }
}
