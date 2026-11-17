package dev.korokmote.app.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.data.Host
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.HexDialog
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.SectionHeader
import dev.korokmote.app.design.Signal
import dev.korokmote.app.design.SignalDot
import dev.korokmote.app.design.TerminalButton
import dev.korokmote.app.design.TerminalRow
import dev.korokmote.app.net.ConnState
import dev.korokmote.app.net.WakeOnLan
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.text.DateFormat
import java.util.Date
import kotlin.random.Random

@Composable
fun HostsScreen(app: KorokApp, nav: Nav) {
    val c = LocalMatrix.current.colors
    val hosts by app.hosts.hosts.collectAsState()
    val found by app.discovery.found.collectAsState()
    val state by app.connection.state.collectAsState()
    var menuFor by remember { mutableStateOf<Host?>(null) }
    val scope = rememberCoroutineScope()

    DisposableEffect(Unit) {
        app.discovery.acquire()
        onDispose { app.discovery.release() }
    }
    var now by remember { mutableStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            delay(2000)
            now = System.currentTimeMillis()
        }
    }

    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
        Spacer(Modifier.height(18.dp))
        ScreenHeader("KOROKMOTE", trailing = { dev.korokmote.app.design.BracketAction("⚙", { nav.push(Route.Settings) }) })
        DecodeText("> select a PC", Modifier.padding(start = 8.dp), style = MType.mono, color = c.phosphor)

        LazyColumn(Modifier.weight(1f).fillMaxWidth()) {
            item { SectionHeader("PAIRED") }
            if (hosts.isEmpty()) {
                item {
                    Text(
                        "No PCs yet.\n\n1. Install Korokmote on Windows.\n2. Right-click the green K in the tray → Pair a device.\n3. Tap [ + PAIR ] below and scan the code.",
                        style = MType.body, color = c.phosphor, modifier = Modifier.padding(vertical = 8.dp),
                    )
                }
            }
            itemsIndexed(hosts.sortedByDescending { it.lastConnected }, key = { _, h -> h.id }) { i, h ->
                val seen = found[h.id]
                val current = state.hostOrNull?.id == h.id
                val signal = when {
                    current && state is ConnState.Online -> Signal.ONLINE
                    seen != null && now - seen.seenAt < 20_000 -> Signal.ONLINE
                    h.mac != null -> Signal.ASLEEP
                    else -> Signal.UNKNOWN
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    SignalDot(signal)
                    Spacer(Modifier.width(10.dp))
                    TerminalRow(
                        index = i,
                        title = h.name,
                        subtitle = "${h.address}:${h.port}" + if (h.lastConnected > 0) " :: " + DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(h.lastConnected)) else "",
                        trailing = when {
                            current && state is ConnState.Connecting -> "…"
                            current && state is ConnState.Reconnecting -> "↻"
                            else -> null
                        },
                        onLongClick = { menuFor = h },
                    ) { app.connection.connect(h) }
                }
            }
            val unpaired = found.values.filter { d -> hosts.none { it.id == d.id } }
            if (unpaired.isNotEmpty()) {
                item { SectionHeader("ON THIS NETWORK") }
                itemsIndexed(unpaired, key = { _, d -> "u" + d.id }) { i, d ->
                    TerminalRow(
                        index = i,
                        title = d.name,
                        subtitle = "${d.host}:${d.port}" + if (d.pairing) " :: pairing open" else " :: open pairing on the PC first",
                        titleColor = if (d.pairing) c.bright else c.phosphor,
                    ) { nav.push(Route.Pair(d.host, d.port, manual = true)) }
                }
            }
        }

        ConnectionFooter(app)
        Row(Modifier.fillMaxWidth().padding(vertical = 10.dp)) {
            TerminalButton("+ PAIR", { nav.push(Route.Pair()) }, Modifier.weight(1f), style = ButtonStyle.PRIMARY)
            Spacer(Modifier.width(8.dp))
            TerminalButton("MANUAL", { nav.push(Route.Pair(manual = true)) }, Modifier.weight(1f))
        }
    }

    menuFor?.let { h ->
        HexDialog(h.name, onDismiss = { menuFor = null }) {
            Text("${h.address}:${h.port}\nfp ${h.fingerprint.take(16)}…", style = MType.mono, color = c.phosphor)
            Spacer(Modifier.height(14.dp))
            TerminalButton("CONNECT", { menuFor = null; app.connection.connect(h) }, Modifier.fillMaxWidth(), style = ButtonStyle.PRIMARY)
            Spacer(Modifier.height(8.dp))
            TerminalButton(if (h.mac != null) "WAKE ON LAN" else "WAKE (no MAC known)", {
                val mac = h.mac ?: return@TerminalButton
                scope.launch {
                    val ok = WakeOnLan.send(mac, h.address)
                    menuFor = null
                    if (ok) app.connection.connect(h)
                }
            }, Modifier.fillMaxWidth(), enabled = h.mac != null)
            Spacer(Modifier.height(8.dp))
            TerminalButton("FORGET", { menuFor = null; app.connection.forget(h.id) }, Modifier.fillMaxWidth(), style = ButtonStyle.DANGER)
        }
    }
}

/** Shows connection progress with a rolling hex stream, and a cancel action. */
@Composable
private fun ConnectionFooter(app: KorokApp) {
    val c = LocalMatrix.current.colors
    val state by app.connection.state.collectAsState()
    var hex by remember { mutableStateOf("") }
    val busy = state is ConnState.Connecting || state is ConnState.Reconnecting
    LaunchedEffect(busy) {
        while (busy) {
            hex = (0 until 8).joinToString(" ") { "%02X".format(Random.nextInt(256)) }
            delay(70)
        }
    }
    if (!busy) return
    val s = state
    Column(Modifier.fillMaxWidth().padding(top = 8.dp)) {
        Text(
            when (s) {
                is ConnState.Connecting -> "> connecting to ${s.host.name}"
                is ConnState.Reconnecting -> "> reconnecting to ${s.host.name} (#${s.attempt}) :: ${s.reason}"
                else -> ""
            },
            style = MType.mono, color = if (s is ConnState.Reconnecting) c.warn else c.bright,
        )
        Text(hex, style = MType.mono, color = c.phosphor)
        Spacer(Modifier.height(6.dp))
        TerminalButton("CANCEL", { app.connection.disconnect() }, Modifier.fillMaxWidth(), minHeight = 44.dp)
    }
}
