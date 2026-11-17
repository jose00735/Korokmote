package dev.korokmote.app.ui

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import dev.korokmote.app.BuildConfig
import dev.korokmote.app.KorokApp
import dev.korokmote.app.data.Host
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.PromptField
import dev.korokmote.app.design.SectionHeader
import dev.korokmote.app.design.TerminalButton
import dev.korokmote.app.design.rememberHaptics
import dev.korokmote.app.net.PairPayload
import dev.korokmote.app.net.PairResult
import dev.korokmote.app.net.Pairing
import dev.korokmote.app.protocol.DeviceInfo
import dev.korokmote.app.protocol.Phrase
import dev.korokmote.app.protocol.Protocol
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private data class Pending(val result: PairResult, val payload: PairPayload?, val phrase: String, val manual: Boolean)

@Composable
fun PairScreen(app: KorokApp, nav: Nav, route: Route.Pair) {
    val ctx = LocalContext.current
    val c = LocalMatrix.current.colors
    val haptics = rememberHaptics()
    val scope = rememberCoroutineScope()
    var manual by remember { mutableStateOf(route.manual) }
    var host by remember { mutableStateOf(route.host.orEmpty()) }
    var port by remember { mutableStateOf((route.port ?: Protocol.DEFAULT_PORT).toString()) }
    var code by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    var pending by remember { mutableStateOf<Pending?>(null) }
    var camGranted by remember {
        mutableStateOf(ContextCompat.checkSelfPermission(ctx, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
    }
    val camLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { camGranted = it }
    LaunchedEffect(manual) { if (!manual && !camGranted) camLauncher.launch(Manifest.permission.CAMERA) }

    val device = DeviceInfo(app.hosts.deviceId, "${Build.MANUFACTURER} ${Build.MODEL}".trim().take(40), "android", BuildConfig.VERSION_NAME)

    fun runPair(payload: PairPayload?) {
        if (busy) return
        busy = true
        error = null
        scope.launch {
            try {
                val hosts = payload?.hosts ?: listOf(host.trim())
                val p = payload?.port ?: port.toIntOrNull() ?: Protocol.DEFAULT_PORT
                val r = Pairing.pair(hosts, p, payload?.fingerprint, payload?.code ?: code.trim(), device)
                val phrase = Phrase.derive(r.fingerprint, device.id)
                if (r.response.phrase.isNotEmpty() && r.response.phrase != phrase) {
                    error = "Verification failed: the reply did not come from the PC that showed the code."
                } else {
                    haptics.medium()
                    pending = Pending(r, payload, phrase, payload?.fingerprint == null)
                }
            } catch (e: Exception) {
                haptics.longLow()
                error = e.message ?: "Pairing failed"
            } finally {
                busy = false
            }
        }
    }

    fun save(p: Pending) {
        val r = p.result
        val h = Host(
            id = r.response.daemon.id.ifEmpty { p.payload?.id ?: r.fingerprint.take(16) },
            name = r.response.daemon.name.ifEmpty { p.payload?.name ?: r.host },
            address = r.host,
            port = r.port,
            fingerprint = r.fingerprint,
            mac = p.payload?.mac,
            tokenEnc = app.hosts.encryptToken(r.response.token),
        )
        app.hosts.upsert(h)
        app.connection.connect(h)
        nav.pendingPair = null
    }

    LaunchedEffect(nav.pendingPair) { nav.pendingPair?.let { runPair(it) } }

    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp).verticalScroll(rememberScrollState())) {
        Spacer(Modifier.height(12.dp))
        ScreenHeader("PAIR", onBack = { nav.pop() })
        val pend = pending
        if (pend != null) {
            SectionHeader("VERIFY")
            Text(if (pend.manual) "Does the PC show these three words?" else "The PC shows the same three words in its notification:", style = MType.body, color = c.phosphor)
            Spacer(Modifier.height(16.dp))
            var shownWords by remember { mutableStateOf(0) }
            LaunchedEffect(pend) {
                for (i in 1..3) {
                    delay(260)
                    shownWords = i
                }
            }
            pend.phrase.split("-").take(shownWords).forEach { w ->
                DecodeText(w.uppercase(), Modifier.padding(vertical = 4.dp), style = MType.display, color = c.head)
            }
            Spacer(Modifier.height(10.dp))
            Text("${pend.result.response.daemon.name} :: ${pend.result.host}:${pend.result.port}", style = MType.mono, color = c.phosphor)
            Spacer(Modifier.height(20.dp))
            if (pend.manual) {
                Row {
                    TerminalButton("YES, CONNECT", { save(pend) }, Modifier.weight(1f), style = ButtonStyle.PRIMARY)
                    Spacer(Modifier.width(8.dp))
                    TerminalButton("NO", { pending = null; error = "Pairing cancelled. Revoke the device on the PC." }, Modifier.weight(1f), style = ButtonStyle.DANGER)
                }
            } else {
                TerminalButton("CONNECT", { save(pend) }, Modifier.fillMaxWidth(), style = ButtonStyle.PRIMARY)
            }
            return@Column
        }

        Row(Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
            TerminalButton("SCAN QR", { manual = false }, Modifier.weight(1f), latched = !manual)
            Spacer(Modifier.width(8.dp))
            TerminalButton("TYPE CODE", { manual = true }, Modifier.weight(1f), latched = manual)
        }

        if (!manual) {
            Text("On the PC: tray icon → Pair a device…", style = MType.mono, color = c.phosphor)
            Spacer(Modifier.height(10.dp))
            if (camGranted) {
                Box(Modifier.fillMaxWidth().aspectRatio(1f)) {
                    QrScanner(onResult = { text ->
                        val p = PairPayload.parse(text)
                        if (p == null) error = "That QR code is not a Korokmote pairing code." else runPair(p)
                    }, Modifier.fillMaxSize(), locked = busy)
                }
            } else {
                Text("Camera permission is needed to scan. You can also type the code.", style = MType.body, color = c.warn)
                Spacer(Modifier.height(8.dp))
                TerminalButton("ALLOW CAMERA", { camLauncher.launch(Manifest.permission.CAMERA) }, Modifier.fillMaxWidth())
            }
        } else {
            SectionHeader("ADDRESS")
            PromptField(host, { host = it }, placeholder = "192.168.1.20", keyboardType = KeyboardType.Uri)
            Spacer(Modifier.height(8.dp))
            PromptField(port, { port = it.filter(Char::isDigit).take(5) }, placeholder = "47810", keyboardType = KeyboardType.Number)
            SectionHeader("SHORT CODE")
            PromptField(code, { code = it.uppercase().take(9) }, placeholder = "ABCD-EFGH", onDone = { runPair(null) })
            Spacer(Modifier.height(14.dp))
            TerminalButton(if (busy) "PAIRING…" else "PAIR", { runPair(null) }, Modifier.fillMaxWidth(), style = ButtonStyle.PRIMARY, enabled = !busy && host.isNotBlank() && code.length >= 8)
            Spacer(Modifier.height(8.dp))
            Text("The address and short code are on the PC's pairing page.", style = MType.mono, color = c.phosphor)
        }
        if (busy) DecodeText("> negotiating secure channel…", Modifier.padding(top = 12.dp), style = MType.mono)
        error?.let { Text("> $it", style = MType.mono, color = c.danger, modifier = Modifier.padding(top = 12.dp)) }
        Spacer(Modifier.height(24.dp))
    }
}
