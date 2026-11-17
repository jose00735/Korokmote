package dev.korokmote.app.protocol

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.security.MessageDigest

/** Korokmote protocol v1. See docs/PROTOCOL.md. */
object Protocol {
    const val VERSION = 1
    const val DEFAULT_PORT = 47810
    const val BEACON_PORT = 47811
    const val SERVICE_TYPE = "_korokmote._tcp."

    val json = Json {
        ignoreUnknownKeys = true
        encodeDefaults = false
        explicitNulls = false
    }

    fun envelope(t: String, id: Long?, d: JsonElement?): String = buildJsonObject {
        put("t", t)
        if (id != null) put("id", id)
        if (d != null) put("d", d)
    }.toString()
}

@Serializable
data class Envelope(
    val t: String,
    val id: Long? = null,
    val re: Long? = null,
    val d: JsonElement? = null,
)

@Serializable
data class ProtoError(val code: String = "internal", val msg: String = "") 

class ProtocolException(val code: String, message: String) : Exception(message)

@Serializable
data class Hello(val v: Int, val device: String, val token: String, val app: String)

@Serializable
data class DaemonInfo(val id: String = "", val name: String = "", val os: String = "", val version: String = "")

@Serializable
data class Screen(val w: Int = 0, val h: Int = 0)

@Serializable
data class HelloReply(
    val v: Int = 1,
    val daemon: DaemonInfo = DaemonInfo(),
    val caps: List<String> = emptyList(),
    val perms: List<String> = emptyList(),
    val screen: Screen = Screen(),
    val locked: Boolean = false,
)

@Serializable
data class AudioDevice(
    val id: String = "",
    val name: String = "",
    val default: Boolean = false,
    val level: Double = 0.0,
    val muted: Boolean = false,
)

@Serializable
data class AudioEvent(val device: String = "default", val level: Double = 0.0, val muted: Boolean = false)

@Serializable
data class WindowInfo(val h: Long = 0, val title: String = "", val exe: String = "")

@Serializable
data class AppInfo(val id: String = "", val name: String = "")

@Serializable
data class FsEntry(
    val name: String = "",
    val path: String = "",
    val dir: Boolean = false,
    val size: Long = 0,
    val mtime: Long = 0,
)

@Serializable
data class RemoteSummary(
    val id: String = "",
    val name: String = "",
    val icon: String? = null,
    val match: List<String>? = null,
)

@Serializable
data class ForegroundEvent(val title: String = "", val exe: String = "", val remote: String? = null)

@Serializable
data class DeviceInfo(val id: String, val name: String, val platform: String, val app: String)

@Serializable
data class PairRequest(val code: String, val device: DeviceInfo)

@Serializable
data class PairResponse(val token: String, val daemon: DaemonInfo = DaemonInfo(), val phrase: String = "")

@Serializable
data class BeaconReply(
    val id: String = "",
    val name: String = "",
    val v: Int = 1,
    val fp: String = "",
    val pair: Boolean = false,
    val port: Int = Protocol.DEFAULT_PORT,
)

/** Binary input frames (PROTOCOL.md section 6). */
object Frames {
    const val OP_MOVE: Byte = 0x01
    const val OP_SCROLL: Byte = 0x02
    const val OP_ABS: Byte = 0x03
    const val OP_BUTTON: Byte = 0x04

    fun rel(op: Byte, dx: Int, dy: Int, seq: Int): ByteArray =
        ByteBuffer.allocate(6).order(ByteOrder.LITTLE_ENDIAN).apply {
            put(op)
            putShort(dx.coerceIn(-32768, 32767).toShort())
            putShort(dy.coerceIn(-32768, 32767).toShort())
            put(seq.toByte())
        }.array()

    fun abs(x: Int, y: Int, seq: Int): ByteArray =
        ByteBuffer.allocate(6).order(ByteOrder.LITTLE_ENDIAN).apply {
            put(OP_ABS)
            putShort(x.coerceIn(0, 65535).toShort())
            putShort(y.coerceIn(0, 65535).toShort())
            put(seq.toByte())
        }.array()

    fun button(button: Int, down: Boolean): ByteArray =
        byteArrayOf(OP_BUTTON, button.toByte(), if (down) 1 else 0)

    data class Decoded(val op: Byte, val x: Int, val y: Int, val seq: Int, val button: Int, val down: Boolean)

    fun decode(b: ByteArray): Decoded? {
        if (b.isEmpty()) return null
        val buf = ByteBuffer.wrap(b).order(ByteOrder.LITTLE_ENDIAN)
        return when (b[0]) {
            OP_MOVE, OP_SCROLL -> if (b.size < 6) null else
                Decoded(b[0], buf.getShort(1).toInt(), buf.getShort(3).toInt(), b[5].toInt() and 0xFF, 0, false)
            OP_ABS -> if (b.size < 6) null else
                Decoded(b[0], buf.getShort(1).toInt() and 0xFFFF, buf.getShort(3).toInt() and 0xFFFF, b[5].toInt() and 0xFF, 0, false)
            OP_BUTTON -> if (b.size < 3) null else Decoded(b[0], 0, 0, 0, b[1].toInt(), b[2].toInt() != 0)
            else -> null
        }
    }
}

/** Verification phrase: words[sha[0]]-words[sha[1]]-words[sha[2]], sha = SHA-256(fp_hex + deviceId). */
object Phrase {
    @Volatile
    var words: List<String> = emptyList()

    fun derive(fpHex: String, deviceId: String, list: List<String> = words): String {
        require(list.size == 256) { "wordlist not loaded" }
        val sha = MessageDigest.getInstance("SHA-256").digest((fpHex.lowercase() + deviceId).toByteArray())
        return (0 until 3).joinToString("-") { list[sha[it].toInt() and 0xFF] }
    }
}

fun JsonObject.str(key: String): String? = (this[key] as? kotlinx.serialization.json.JsonPrimitive)?.let {
    if (it.isString) it.content else it.content
}

fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }

fun sha256Hex(b: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(b).toHex()
