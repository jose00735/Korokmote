package dev.korokmote.app.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.DigitalRain
import dev.korokmote.app.design.HexDialog
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.MatrixTheme
import dev.korokmote.app.design.RainBus
import dev.korokmote.app.design.RainMood
import dev.korokmote.app.design.TerminalButton
import dev.korokmote.app.design.crt
import dev.korokmote.app.design.glitch
import dev.korokmote.app.design.rememberHaptics
import dev.korokmote.app.design.rememberOneShot
import dev.korokmote.app.design.sweep
import dev.korokmote.app.net.ConnState
import dev.korokmote.app.net.Pulse
import kotlinx.coroutines.delay

@Composable
fun AppRoot(app: KorokApp, nav: Nav) {
    val settings by app.settings.state.collectAsState()
    MatrixTheme(settings) {
        val cfg = LocalMatrix.current
        val c = cfg.colors
        val state by app.connection.state.collectAsState()
        val route = nav.current
        val haptics = rememberHaptics()
        val glitch = rememberOneShot(route, cfg.dur(160))
        var connectedCount by remember { mutableIntStateOf(0) }
        val sweep = rememberOneShot(connectedCount, cfg.dur(220))
        var failure by remember { mutableStateOf<ConnState.Failed?>(null) }
        var lastTouch by remember { mutableLongStateOf(System.currentTimeMillis()) }
        var idle by remember { mutableStateOf(false) }

        // Connection moments drive the rain, haptics, sweep and routing.
        LaunchedEffect(Unit) {
            app.connection.pulse.collect { p ->
                when (p) {
                    Pulse.CONNECTED -> {
                        connectedCount++
                        haptics.medium()
                        RainBus.mood.value = RainMood.FAST
                        delay(1200)
                        RainBus.mood.value = RainMood.NORMAL
                    }
                    Pulse.LOST -> {
                        haptics.longLow()
                        RainBus.mood.value = RainMood.FROZEN
                        delay(400)
                        RainBus.mood.value = RainMood.DRAIN
                        delay(1600)
                        RainBus.mood.value = RainMood.SLOW
                    }
                    Pulse.POWER -> Unit
                }
            }
        }
        LaunchedEffect(state) {
            when (val s = state) {
                is ConnState.Connecting -> RainBus.mood.value = RainMood.FAST
                is ConnState.Online -> {
                    if (RainBus.mood.value == RainMood.SLOW) RainBus.mood.value = RainMood.NORMAL
                    if (nav.current is Route.Hosts || nav.current is Route.Boot || nav.current is Route.Pair) nav.replaceAll(Route.Main(Tab.PAD))
                }
                is ConnState.Failed -> {
                    RainBus.mood.value = RainMood.NORMAL
                    failure = s
                    if (nav.current !is Route.Hosts && nav.current !is Route.Pair) nav.replaceAll(Route.Hosts)
                }
                ConnState.Idle -> {
                    RainBus.mood.value = RainMood.NORMAL
                    if (nav.current !is Route.Hosts && nav.current !is Route.Pair && nav.current !is Route.Boot && nav.current !is Route.Settings) nav.replaceAll(Route.Hosts)
                }
                is ConnState.Reconnecting -> Unit
            }
        }
        LaunchedEffect(nav.pendingPair) {
            if (nav.pendingPair != null && nav.current !is Route.Pair) nav.push(Route.Pair())
        }
        // Standby dimming after 60 s without touch on control screens (keeps the screen on).
        val view = LocalView.current
        val keepOn = settings.keepScreenOn && state is ConnState.Online && (route is Route.Main || route is Route.Remote)
        LaunchedEffect(keepOn) { view.keepScreenOn = keepOn }
        LaunchedEffect(keepOn, lastTouch) {
            idle = false
            if (keepOn) {
                delay(60_000)
                idle = true
            }
        }

        val rainAlpha = when (route) {
            Route.Boot -> 1f
            Route.Hosts, is Route.Pair -> 0.6f
            is Route.Main -> if (route.tab == Tab.PAD || route.tab == Tab.KEYS) 0.15f else 0.35f
            is Route.Remote -> 0.15f
            else -> 0.35f
        }
        Box(
            Modifier
                .fillMaxSize()
                .background(c.void)
                .pointerInput(Unit) {
                    awaitPointerEventScope {
                        while (true) {
                            awaitPointerEvent(PointerEventPass.Initial)
                            lastTouch = System.currentTimeMillis()
                        }
                    }
                }
                .crt(cfg),
        ) {
            DigitalRain(Modifier.fillMaxSize(), alpha = if (idle) 0.25f else rainAlpha)
            Box(
                Modifier
                    .fillMaxSize()
                    .glitch({ glitch.value }, cfg)
                    .sweep({ sweep.value }, c.bright)
                    .systemBarsPadding()
                    .imePadding(),
            ) {
                when (route) {
                    Route.Boot -> BootScreen(app, nav)
                    Route.Hosts -> HostsScreen(app, nav)
                    is Route.Pair -> PairScreen(app, nav, route)
                    is Route.Main -> MainScreen(app, nav, route.tab)
                    is Route.Remote -> RemoteScreen(app, nav, route.id)
                    Route.Power -> PowerScreen(app, nav)
                    is Route.Files -> FilesScreen(app, nav, route.tab)
                    Route.Clipboard -> ClipboardScreen(app, nav)
                    Route.Settings -> SettingsScreen(app, nav)
                }
                ErrorTicker(app)
            }
            if (idle) Box(Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.7f)))
        }
        BackHandler(enabled = nav.stack.size > 1) { nav.pop() }
        failure?.let { f ->
            HexDialog("CONNECTION", onDismiss = { failure = null }) {
                Text(f.reason, style = MType.body, color = c.bright)
                Spacer(Modifier.height(16.dp))
                Row {
                    if (f.needsPairing) {
                        TerminalButton("PAIR AGAIN", {
                            failure = null
                            app.connection.forget(f.host.id)
                            nav.replaceAll(Route.Hosts)
                            nav.push(Route.Pair(f.host.address, f.host.port))
                        }, Modifier.weight(1f), style = ButtonStyle.PRIMARY)
                        Spacer(Modifier.width(8.dp))
                    }
                    TerminalButton("CLOSE", { failure = null; app.connection.disconnect() }, Modifier.weight(1f))
                }
            }
        }
    }
}

/** Shows request errors from the connection briefly at the bottom. */
@Composable
private fun ErrorTicker(app: KorokApp) {
    val c = LocalMatrix.current.colors
    var msg by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(Unit) {
        app.connection.errors.collect {
            msg = it
            delay(2600)
            msg = null
        }
    }
    msg?.let {
        Box(Modifier.fillMaxSize().padding(16.dp), contentAlignment = Alignment.BottomCenter) {
            Text(
                "> error :: $it",
                style = MType.mono,
                color = c.danger,
                modifier = Modifier.fillMaxWidth().background(c.void).padding(12.dp),
            )
        }
    }
}

/** `> DESKTOP-JOSE :: online :: 3 ms` with state colours and a [ LOCKED ] tag. */
@Composable
fun StatusLine(app: KorokApp, modifier: Modifier = Modifier) {
    val c = LocalMatrix.current.colors
    val state by app.connection.state.collectAsState()
    val rtt by app.connection.rtt.collectAsState()
    val locked by app.connection.locked.collectAsState()
    val (text, color) = when (val s = state) {
        is ConnState.Online -> "> ${s.host.name} :: online" + (if (rtt >= 0) " :: $rtt ms" else "") + (if (locked) " :: [ LOCKED ]" else "") to c.bright
        is ConnState.Connecting -> "> ${s.host.name} :: connecting" to c.bright
        is ConnState.Reconnecting -> "> ${s.host.name} :: reconnecting #${s.attempt}" to c.warn
        is ConnState.Failed -> "> ${s.host.name} :: ${s.reason}" to c.danger
        ConnState.Idle -> "> offline" to c.phosphor
    }
    Row(
        modifier
            .fillMaxWidth()
            .drawBehind { drawLine(c.dim, Offset(0f, size.height), Offset(size.width, size.height), 1f) }
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        DecodeText(text, Modifier.weight(1f), style = MType.mono, color = color, maxLines = 1)
    }
}

/** Bottom command bar: `[PAD] [KEYS] [MEDIA] [REMOTES] [MORE]`. */
@Composable
fun CommandBar(selected: Tab, onSelect: (Tab) -> Unit) {
    val c = LocalMatrix.current.colors
    val haptics = rememberHaptics()
    Row(
        Modifier
            .fillMaxWidth()
            .drawBehind { drawLine(c.dim, Offset(0f, 0f), Offset(size.width, 0f), 1f) }
            .padding(vertical = 4.dp),
    ) {
        Tab.entries.forEach { t ->
            val active = t == selected
            Column(
                Modifier
                    .weight(1f)
                    .pointerInput(t) {
                        awaitPointerEventScope {
                            while (true) {
                                val e = awaitPointerEvent()
                                if (e.changes.any { it.changedToUp() }) {
                                    haptics.tick()
                                    onSelect(t)
                                }
                            }
                        }
                    }
                    .padding(vertical = 12.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Text(
                    if (active) "[${t.name}]" else t.name,
                    style = MType.micro,
                    color = if (active) c.bright else c.phosphor,
                    maxLines = 1,
                    overflow = TextOverflow.Clip,
                )
                Box(
                    Modifier
                        .padding(top = 4.dp)
                        .width(24.dp)
                        .height(2.dp)
                        .background(if (active) c.bright else Color.Transparent),
                )
            }
        }
    }
}

private fun androidx.compose.ui.input.pointer.PointerInputChange.changedToUp() = !pressed && previousPressed
