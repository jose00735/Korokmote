package dev.korokmote.app.ui

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.HoldRing
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.SectionHeader
import dev.korokmote.app.design.TerminalButton
import dev.korokmote.app.net.WakeOnLan
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

@Composable
fun PowerScreen(app: KorokApp, nav: Nav) {
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val scope = rememberCoroutineScope()
    val scaleY = remember { Animatable(1f) }
    val scaleX = remember { Animatable(1f) }
    var status by remember { mutableStateOf<String?>(null) }
    val state by app.connection.state.collectAsState()

    fun run(action: String, endsSession: Boolean, delaySec: Int = 0) {
        scope.launch {
            try {
                app.connection.request("power", Msg.power(action, delaySec))
                status = "> $action sent"
                if (endsSession && delaySec == 0) {
                    // "Power down": collapse to a line, then a dot (DESIGN.md 5.7).
                    if (cfg.animated) {
                        scaleY.animateTo(0.004f, tween(cfg.dur(160)))
                        scaleX.animateTo(0.01f, tween(cfg.dur(120)))
                        delay(cfg.dur(80).toLong())
                    }
                    app.connection.disconnect()
                    nav.replaceAll(Route.Hosts)
                }
            } catch (e: Exception) {
                status = "> error :: ${e.message}"
            }
        }
    }

    Column(
        Modifier
            .fillMaxSize()
            .graphicsLayer {
                this.scaleY = scaleY.value
                this.scaleX = scaleX.value
            },
    ) {
        StatusLine(app)
        ScreenHeader("POWER", onBack = { nav.pop() })
        Column(Modifier.fillMaxSize().padding(horizontal = 14.dp).verticalScroll(rememberScrollState())) {
            Text("Press and hold to confirm.", style = MType.mono, color = c.phosphor)
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                HoldRing("LOCK", { run("lock", false) })
                HoldRing("SCREEN OFF", { run("monitor_off", false) })
            }
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                HoldRing("SLEEP", { run("sleep", true) })
                HoldRing("HIBERNATE", { run("hibernate", true) })
            }
            Spacer(Modifier.height(14.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceEvenly) {
                HoldRing("RESTART", { run("restart", true) }, danger = true)
                HoldRing("SHUT DOWN", { run("shutdown", true) }, danger = true)
            }
            SectionHeader("TIMED")
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TerminalButton("SHUT DOWN IN 60 s", { run("shutdown", true, 60) }, Modifier.weight(1f))
                TerminalButton("ABORT", { run("abort", false) }, Modifier.weight(1f))
            }
            state.hostOrNull?.mac?.let { mac ->
                SectionHeader("WAKE")
                TerminalButton("SEND WAKE-ON-LAN", {
                    scope.launch { status = if (WakeOnLan.send(mac, state.hostOrNull?.address)) "> magic packet sent to $mac" else "> could not send" }
                }, Modifier.fillMaxWidth())
            }
            status?.let { Text(it, style = MType.mono, color = c.bright, modifier = Modifier.padding(vertical = 12.dp)) }
            Spacer(Modifier.height(20.dp))
        }
    }
}
