package dev.korokmote.app.net

import android.net.Uri
import dev.korokmote.app.protocol.DeviceInfo
import dev.korokmote.app.protocol.PairRequest
import dev.korokmote.app.protocol.PairResponse
import dev.korokmote.app.protocol.ProtoError
import dev.korokmote.app.protocol.Protocol
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/** Parsed korokmote://pair URI from the QR code. */
data class PairPayload(
    val id: String,
    val name: String,
    val hosts: List<String>,
    val port: Int,
    val fingerprint: String?,
    val code: String,
    val mac: String?,
) {
    companion object {
        fun parse(text: String): PairPayload? {
            val uri = runCatching { Uri.parse(text.trim()) }.getOrNull() ?: return null
            if (uri.scheme != "korokmote" || uri.host != "pair") return null
            val code = uri.getQueryParameter("c") ?: return null
            val hosts = uri.getQueryParameter("h")?.split(",")?.map { it.trim() }?.filter { it.isNotEmpty() }.orEmpty()
            if (hosts.isEmpty()) return null
            val fp = uri.getQueryParameter("fp")?.lowercase()
            if (fp != null && !Regex("^[0-9a-f]{64}$").matches(fp)) return null
            return PairPayload(
                id = uri.getQueryParameter("id").orEmpty(),
                name = uri.getQueryParameter("name") ?: "PC",
                hosts = hosts,
                port = uri.getQueryParameter("p")?.toIntOrNull() ?: Protocol.DEFAULT_PORT,
                fingerprint = fp,
                code = code,
                mac = uri.getQueryParameter("mac")?.takeIf { it.isNotBlank() },
            )
        }
    }
}

data class PairResult(val response: PairResponse, val host: String, val port: Int, val fingerprint: String)

class PairingException(message: String) : Exception(message)

object Pairing {
    private val jsonType = "application/json".toMediaType()

    /** Tries each host in order. With fingerprint == null the certificate is learned (manual pairing). */
    suspend fun pair(hosts: List<String>, port: Int, fingerprint: String?, code: String, device: DeviceInfo): PairResult =
        withContext(Dispatchers.IO) {
            var last: Exception? = null
            for (host in hosts) {
                try {
                    val (client, tm) = Http.pinned(fingerprint)
                    val body = Protocol.json.encodeToString(PairRequest.serializer(), PairRequest(code, device))
                    val req = Request.Builder()
                        .url("https://${hostForUrl(host)}:$port/v1/pair")
                        .post(body.toRequestBody(jsonType))
                        .build()
                    client.newCall(req).execute().use { resp ->
                        val text = resp.body?.string().orEmpty()
                        if (!resp.isSuccessful) {
                            val err = runCatching { Protocol.json.decodeFromString(ProtoError.serializer(), text) }.getOrNull()
                            throw PairingException(err?.msg?.takeIf { it.isNotBlank() } ?: "Pairing refused (${resp.code})")
                        }
                        val pr = Protocol.json.decodeFromString(PairResponse.serializer(), text)
                        val fp = tm.seen ?: throw PairingException("No certificate seen")
                        return@withContext PairResult(pr, host, port, fp)
                    }
                } catch (e: PairingException) {
                    throw e
                } catch (e: Exception) {
                    last = e
                }
            }
            throw PairingException(last?.message?.let { "Could not reach the PC: $it" } ?: "Could not reach the PC")
        }

    fun hostForUrl(host: String) = if (host.contains(':') && !host.startsWith("[")) "[$host]" else host
}
