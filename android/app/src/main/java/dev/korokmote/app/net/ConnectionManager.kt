package dev.korokmote.app.net

import android.content.Context
import android.content.Intent
import android.content.pm.ShortcutInfo
import android.content.pm.ShortcutManager
import android.graphics.drawable.Icon
import android.net.ConnectivityManager
import android.net.Network
import dev.korokmote.app.BuildConfig
import dev.korokmote.app.MainActivity
import dev.korokmote.app.R
import dev.korokmote.app.data.Host
import dev.korokmote.app.data.HostStore
import dev.korokmote.app.protocol.AudioEvent
import dev.korokmote.app.protocol.Envelope
import dev.korokmote.app.protocol.ForegroundEvent
import dev.korokmote.app.protocol.HelloReply
import dev.korokmote.app.protocol.Protocol
import dev.korokmote.app.protocol.ProtocolException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.BufferOverflow
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive

sealed interface ConnState {
    data object Idle : ConnState
    data class Connecting(val host: Host, val attempt: Int) : ConnState
    data class Online(val host: Host, val hello: HelloReply) : ConnState
    data class Reconnecting(val host: Host, val attempt: Int, val reason: String) : ConnState
    data class Failed(val host: Host, val reason: String, val needsPairing: Boolean) : ConnState

    val hostOrNull: Host?
        get() = when (this) {
            Idle -> null
            is Connecting -> host
            is Online -> host
            is Reconnecting -> host
            is Failed -> host
        }
}

enum class Pulse { CONNECTED, LOST, POWER }

/** Owns the single live connection, reconnects with backoff and exposes state to the UI. */
class ConnectionManager(
    private val context: Context,
    private val scope: CoroutineScope,
    private val hosts: HostStore,
    private val discovery: Discovery,
) {
    private val _state = MutableStateFlow<ConnState>(ConnState.Idle)
    val state: StateFlow<ConnState> = _state
    private val _events = MutableSharedFlow<Envelope>(extraBufferCapacity = 64, onBufferOverflow = BufferOverflow.DROP_OLDEST)
    val events: SharedFlow<Envelope> = _events
    private val _pulse = MutableSharedFlow<Pulse>(extraBufferCapacity = 8, onBufferOverflow = BufferOverflow.DROP_OLDEST)
    val pulse: SharedFlow<Pulse> = _pulse
    val rtt = MutableStateFlow(-1L)
    val locked = MutableStateFlow(false)
    val audio = MutableStateFlow<AudioEvent?>(null)
    val foreground = MutableStateFlow<ForegroundEvent?>(null)

    @Volatile var conn: KorokConnection? = null
        private set
    private var loop: Job? = null
    private val kick = Channel<Unit>(Channel.CONFLATED)
    private var discoveryHeld = false

    init {
        val cm = context.getSystemService(ConnectivityManager::class.java)
        runCatching {
            cm.registerDefaultNetworkCallback(object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    kick.trySend(Unit)
                }
            })
        }
    }

    private fun holdDiscovery(on: Boolean) {
        if (on == discoveryHeld) return
        discoveryHeld = on
        if (on) discovery.acquire() else discovery.release()
    }

    fun connect(host: Host) {
        loop?.cancel()
        conn?.close()
        conn = null
        hosts.lastHostId = host.id
        loop = scope.launch { run(host.id) }
    }

    fun disconnect() {
        loop?.cancel()
        loop = null
        conn?.close()
        conn = null
        holdDiscovery(false)
        _state.value = ConnState.Idle
    }

    /** Forgets the stored host and stops if it was active. */
    fun forget(id: String) {
        if (_state.value.hostOrNull?.id == id) disconnect()
        hosts.remove(id)
    }

    private suspend fun run(hostId: String) {
        var attempt = 0
        while (scope.isActive) {
            var host = hosts.get(hostId) ?: run {
                _state.value = ConnState.Idle
                return
            }
            // Prefer the address discovery saw most recently (DHCP may have moved the PC).
            discovery.found.value[host.id]?.let { d ->
                if (d.host != host.address || d.port != host.port) {
                    hosts.update(host.id) { it.copy(address = d.host, port = d.port) }
                    host = hosts.get(hostId)!!
                }
            }
            _state.value = if (attempt == 0) ConnState.Connecting(host, 0) else ConnState.Reconnecting(host, attempt, (_state.value as? ConnState.Reconnecting)?.reason ?: "connection lost")
            holdDiscovery(true)
            val token = hosts.token(host)
            if (token == null) {
                _state.value = ConnState.Failed(host, "Stored credentials are unreadable. Pair again.", true)
                holdDiscovery(false)
                return
            }
            val closed = Channel<String?>(1)
            val c = KorokConnection(scope, host.address, host.port, host.fingerprint, hosts.deviceId, token, BuildConfig.VERSION_NAME) { _, t, reason ->
                closed.trySend(t?.message ?: reason)
            }
            try {
                val hello = c.open()
                conn = c
                attempt = 0
                locked.value = hello.locked
                hosts.update(host.id) { it.copy(lastConnected = System.currentTimeMillis(), name = hello.daemon.name.ifBlank { it.name }) }
                host = hosts.get(hostId) ?: host
                _state.value = ConnState.Online(host, hello)
                _pulse.tryEmit(Pulse.CONNECTED)
                holdDiscovery(false)
                publishShortcut(host)
                refreshAudio()
                val relay = scope.launch { c.events.collect { onEvent(it) } }
                val pinger = scope.launch {
                    while (isActive) {
                        c.ping()
                        delay(5000)
                        rtt.value = c.rttMs
                        if (System.currentTimeMillis() - c.lastPongAt > 16000) c.close()
                    }
                }
                val reason = closed.receive()
                relay.cancel()
                pinger.cancel()
                conn = null
                if (_state.value is ConnState.Failed) return
                _pulse.tryEmit(Pulse.LOST)
                _state.value = ConnState.Reconnecting(host, 1, reason ?: "connection lost")
            } catch (e: ProtocolException) {
                c.close()
                if (e.code == "unauthorized") {
                    _state.value = ConnState.Failed(host, "This PC no longer recognises this phone. Pair again.", true)
                    holdDiscovery(false)
                    return
                }
                _state.value = ConnState.Reconnecting(host, attempt + 1, e.message ?: "error")
            } catch (e: kotlinx.coroutines.CancellationException) {
                c.close()
                throw e
            } catch (e: Exception) {
                c.close()
                val msg = when {
                    e is javax.net.ssl.SSLHandshakeException || e.cause is java.security.cert.CertificateException ->
                        "Certificate mismatch: this is not the paired PC"
                    else -> e.message ?: e.javaClass.simpleName
                }
                _state.value = ConnState.Reconnecting(host, attempt + 1, msg)
            }
            attempt++
            val wait = (250L shl (attempt - 1).coerceAtMost(5)).coerceAtMost(8000)
            discovery.probe()
            withTimeoutOrNull(wait) { kick.receive() }
        }
    }

    private fun onEvent(env: Envelope) {
        when (env.t) {
            "ev.audio" -> env.d?.let { audio.value = Protocol.json.decodeFromJsonElement(AudioEvent.serializer(), it) }
            "ev.lock" -> locked.value = (env.d as? JsonObject)?.get("locked")?.jsonPrimitive?.content == "true"
            "ev.fg" -> env.d?.let { foreground.value = Protocol.json.decodeFromJsonElement(ForegroundEvent.serializer(), it) }
            "bye" -> {
                val reason = (env.d as? JsonObject)?.get("reason")?.jsonPrimitive?.content
                if (reason == "revoked") {
                    val h = _state.value.hostOrNull
                    if (h != null) _state.value = ConnState.Failed(h, "This phone was removed on the PC. Pair again.", true)
                }
            }
        }
        _events.tryEmit(env)
    }

    fun refreshAudio() {
        scope.launch {
            runCatching {
                val d = request("audio.list")
                val list = (d as JsonObject)["devices"]
                val devs = Protocol.json.decodeFromJsonElement(kotlinx.serialization.builtins.ListSerializer(dev.korokmote.app.protocol.AudioDevice.serializer()), list!!)
                devs.firstOrNull { it.default }?.let { audio.value = AudioEvent("default", it.level, it.muted) }
            }
        }
    }

    val online: Boolean get() = conn?.isOpen == true && _state.value is ConnState.Online

    suspend fun request(t: String, d: JsonElement? = null, timeoutMs: Long = 8000): JsonElement {
        val c = conn ?: throw ProtocolException("closed", "Not connected")
        return c.request(t, d, timeoutMs)
    }

    /** Fire-and-forget request that reports errors through [errors]. */
    fun fire(t: String, d: JsonElement? = null) {
        scope.launch {
            try {
                request(t, d)
            } catch (e: Exception) {
                _errors.tryEmit(e.message ?: "Request failed")
            }
        }
    }

    private val _errors = MutableSharedFlow<String>(extraBufferCapacity = 8, onBufferOverflow = BufferOverflow.DROP_OLDEST)
    val errors: SharedFlow<String> = _errors

    fun emitPulse(p: Pulse) {
        _pulse.tryEmit(p)
    }

    private fun publishShortcut(host: Host) {
        runCatching {
            val sm = context.getSystemService(ShortcutManager::class.java)
            val intent = Intent(context, MainActivity::class.java)
                .setAction(MainActivity.ACTION_CONNECT)
                .putExtra(MainActivity.EXTRA_HOST, host.id)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            val info = ShortcutInfo.Builder(context, "host-${host.id}")
                .setShortLabel(host.name.take(24))
                .setLongLabel("Control ${host.name}")
                .setIcon(Icon.createWithResource(context, R.mipmap.ic_launcher))
                .setIntent(intent)
                .build()
            sm.pushDynamicShortcut(info)
        }
    }
}
