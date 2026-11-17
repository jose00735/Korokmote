package dev.korokmote.app.net

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import dev.korokmote.app.protocol.BeaconReply
import dev.korokmote.app.protocol.Protocol
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.SocketTimeoutException
import java.util.concurrent.ConcurrentLinkedQueue

/** A daemon seen on the network. */
data class Discovered(
    val id: String,
    val name: String,
    val host: String,
    val port: Int,
    val fpPrefix: String,
    val pairing: Boolean,
    val seenAt: Long = System.currentTimeMillis(),
)

/**
 * Finds daemons with mDNS (NsdManager) and the UDP beacon fallback.
 * Reference-counted: discovery runs while at least one screen or the connection needs it.
 */
class Discovery(private val context: Context, private val scope: CoroutineScope) {
    private val _found = MutableStateFlow<Map<String, Discovered>>(emptyMap())
    val found: StateFlow<Map<String, Discovered>> = _found

    private val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager
    private val wifi = context.applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
    private var lock: WifiManager.MulticastLock? = null
    private var listener: NsdManager.DiscoveryListener? = null
    private var beaconJob: Job? = null
    private var users = 0
    private val resolveQueue = ConcurrentLinkedQueue<NsdServiceInfo>()
    @Volatile private var resolving = false

    @Synchronized
    fun acquire() {
        users++
        if (users == 1) start()
    }

    @Synchronized
    fun release() {
        users = (users - 1).coerceAtLeast(0)
        if (users == 0) stop()
    }

    /** Sends one round of beacon probes right away (used before reconnecting). */
    fun probe() {
        scope.launch(Dispatchers.IO) { beaconRound() }
    }

    private fun put(d: Discovered) {
        _found.value = _found.value + (d.id to d)
    }

    private fun start() {
        lock = wifi.createMulticastLock("korokmote-mdns").apply {
            setReferenceCounted(false)
            runCatching { acquire() }
        }
        val l = object : NsdManager.DiscoveryListener {
            override fun onDiscoveryStarted(serviceType: String?) {}
            override fun onDiscoveryStopped(serviceType: String?) {}
            override fun onStartDiscoveryFailed(serviceType: String?, errorCode: Int) {}
            override fun onStopDiscoveryFailed(serviceType: String?, errorCode: Int) {}
            override fun onServiceLost(info: NsdServiceInfo?) {}
            override fun onServiceFound(info: NsdServiceInfo) {
                resolveQueue.add(info)
                nextResolve()
            }
        }
        listener = l
        runCatching { nsd.discoverServices(Protocol.SERVICE_TYPE, NsdManager.PROTOCOL_DNS_SD, l) }
        beaconJob = scope.launch(Dispatchers.IO) {
            while (isActive) {
                beaconRound()
                delay(4000)
            }
        }
    }

    private fun stop() {
        listener?.let { runCatching { nsd.stopServiceDiscovery(it) } }
        listener = null
        beaconJob?.cancel()
        beaconJob = null
        runCatching { lock?.release() }
        lock = null
    }

    @Suppress("DEPRECATION")
    private fun nextResolve() {
        if (resolving) return
        val info = resolveQueue.poll() ?: return
        resolving = true
        val done = {
            resolving = false
            nextResolve()
        }
        val cb = object : NsdManager.ResolveListener {
            override fun onResolveFailed(serviceInfo: NsdServiceInfo?, errorCode: Int) {
                done()
            }

            override fun onServiceResolved(si: NsdServiceInfo) {
                val attrs = si.attributes.mapValues { (_, v) -> v?.toString(Charsets.UTF_8).orEmpty() }
                val host = if (Build.VERSION.SDK_INT >= 34) {
                    si.hostAddresses.firstOrNull { it is java.net.Inet4Address }?.hostAddress ?: si.hostAddresses.firstOrNull()?.hostAddress
                } else {
                    si.host?.hostAddress
                }
                val id = attrs["id"].orEmpty()
                if (host != null && id.isNotEmpty()) {
                    put(
                        Discovered(
                            id = id,
                            name = attrs["name"]?.takeIf { it.isNotBlank() } ?: si.serviceName,
                            host = host,
                            port = si.port,
                            fpPrefix = attrs["fp"].orEmpty(),
                            pairing = attrs["pair"] == "1",
                        ),
                    )
                }
                done()
            }
        }
        runCatching { nsd.resolveService(info, cb) }.onFailure { done() }
    }

    private fun beaconRound() {
        try {
            DatagramSocket().use { s ->
                s.broadcast = true
                s.soTimeout = 1200
                val probe = byteArrayOf('K'.code.toByte(), 'R'.code.toByte(), 'K'.code.toByte(), 'M'.code.toByte(), '?'.code.toByte(), Protocol.VERSION.toByte(), 0, 0)
                s.send(DatagramPacket(probe, probe.size, InetAddress.getByName("255.255.255.255"), Protocol.BEACON_PORT))
                val buf = ByteArray(2048)
                val end = System.currentTimeMillis() + 1200
                while (System.currentTimeMillis() < end) {
                    val p = DatagramPacket(buf, buf.size)
                    try {
                        s.receive(p)
                    } catch (_: SocketTimeoutException) {
                        break
                    }
                    val reply = runCatching {
                        Protocol.json.decodeFromString(BeaconReply.serializer(), String(p.data, 0, p.length))
                    }.getOrNull() ?: continue
                    if (reply.id.isEmpty()) continue
                    put(Discovered(reply.id, reply.name, p.address.hostAddress ?: continue, reply.port, reply.fp, reply.pair))
                }
            }
        } catch (_: Exception) {
        }
    }
}
