package dev.korokmote.app.net

import dev.korokmote.app.protocol.Envelope
import dev.korokmote.app.protocol.Frames
import dev.korokmote.app.protocol.Hello
import dev.korokmote.app.protocol.HelloReply
import dev.korokmote.app.protocol.ProtoError
import dev.korokmote.app.protocol.Protocol
import dev.korokmote.app.protocol.ProtocolException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong

/**
 * One authenticated WebSocket session with a daemon. Created and owned by [ConnectionManager].
 */
class KorokConnection(
    private val scope: CoroutineScope,
    val host: String,
    val port: Int,
    private val fingerprint: String,
    private val deviceId: String,
    private val token: String,
    private val appVersion: String,
    private val onClosed: (KorokConnection, Throwable?, String?) -> Unit,
) {
    private val nextId = AtomicLong(0)
    private val seq = AtomicInteger(0)
    private val pending = ConcurrentHashMap<Long, CompletableDeferred<Envelope>>()
    private val _events = MutableSharedFlow<Envelope>(extraBufferCapacity = 64, onBufferOverflow = BufferOverflow.DROP_OLDEST)
    val events: SharedFlow<Envelope> = _events
    @Volatile private var ws: WebSocket? = null
    @Volatile private var closed = false
    @Volatile var lastPongAt = System.currentTimeMillis()
        private set
    @Volatile var rttMs: Long = -1
        private set

    /** Opens the socket and performs the hello handshake. */
    suspend fun open(): HelloReply {
        val (client, _) = Http.pinned(fingerprint)
        val opened = CompletableDeferred<Unit>()
        val req = Request.Builder().url("wss://${Pairing.hostForUrl(host)}:$port/v1/ws").build()
        ws = client.newWebSocket(req, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                opened.complete(Unit)
            }

            override fun onMessage(webSocket: WebSocket, text: String) = handle(text)

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {}

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
                finish(null, reason)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) = finish(null, reason)

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                opened.completeExceptionally(t)
                finish(t, null)
            }
        })
        withTimeout(6000) { opened.await() }
        val d = Protocol.json.encodeToJsonElement(Hello.serializer(), Hello(Protocol.VERSION, deviceId, token, appVersion))
        val reply = withTimeout(6000) { request("hello", d) }
        return Protocol.json.decodeFromJsonElement(HelloReply.serializer(), reply)
    }

    private fun handle(text: String) {
        val env = runCatching { Protocol.json.decodeFromString(Envelope.serializer(), text) }.getOrNull() ?: return
        val re = env.re
        if (re != null) {
            pending.remove(re)?.complete(env)
            return
        }
        if (env.t == "pong") {
            lastPongAt = System.currentTimeMillis()
            val ts = (env.d as? JsonObject)?.get("ts")?.toString()?.toLongOrNull()
            if (ts != null) rttMs = (System.currentTimeMillis() - ts).coerceAtLeast(0)
        }
        _events.tryEmit(env)
    }

    private fun finish(t: Throwable?, reason: String?) {
        if (closed) return
        closed = true
        pending.values.forEach { it.completeExceptionally(ProtocolException("closed", "connection closed")) }
        pending.clear()
        scope.launch { onClosed(this@KorokConnection, t, reason) }
    }

    val isOpen get() = !closed && ws != null

    /** Sends a request and waits for ok/err. */
    suspend fun request(t: String, d: JsonElement? = null, timeoutMs: Long = 8000): JsonElement {
        val id = nextId.incrementAndGet()
        val def = CompletableDeferred<Envelope>()
        pending[id] = def
        val sent = ws?.send(Protocol.envelope(t, id, d)) ?: false
        if (!sent) {
            pending.remove(id)
            throw ProtocolException("closed", "not connected")
        }
        val env = try {
            withTimeout(timeoutMs) { def.await() }
        } finally {
            pending.remove(id)
        }
        if (env.t == "err") {
            val e = env.d?.let { runCatching { Protocol.json.decodeFromJsonElement(ProtoError.serializer(), it) }.getOrNull() }
            throw ProtocolException(e?.code ?: "internal", e?.msg ?: "request failed")
        }
        return env.d ?: JsonObject(emptyMap())
    }

    /** Fire-and-forget control message. */
    fun send(t: String, d: JsonElement? = null): Boolean = ws?.send(Protocol.envelope(t, null, d)) ?: false

    fun ping() {
        send("ping", buildJsonObject { put("ts", System.currentTimeMillis()) })
    }

    fun pointer(dx: Int, dy: Int) {
        if (dx == 0 && dy == 0) return
        ws?.send(Frames.rel(Frames.OP_MOVE, dx, dy, seq.incrementAndGet()).toByteString())
    }

    fun scroll(dx: Int, dy: Int) {
        if (dx == 0 && dy == 0) return
        ws?.send(Frames.rel(Frames.OP_SCROLL, dx, dy, seq.incrementAndGet()).toByteString())
    }

    fun button(button: Int, down: Boolean) {
        ws?.send(Frames.button(button, down).toByteString())
    }

    fun close() {
        ws?.close(1000, "bye")
        finish(null, "closed by user")
    }
}
