package dev.korokmote.app.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.PromptField
import dev.korokmote.app.design.SectionHeader
import dev.korokmote.app.design.TerminalButton
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

@Composable
fun ClipboardScreen(app: KorokApp, nav: Nav) {
    val c = LocalMatrix.current.colors
    val ctx = LocalContext.current
    val cm = remember { ctx.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager }
    val scope = rememberCoroutineScope()
    var text by remember { mutableStateOf("") }
    var status by remember { mutableStateOf<String?>(null) }
    var follow by remember { mutableStateOf(false) }

    LaunchedEffect(follow) {
        runCatching { app.connection.request("clip.sub", buildJsonObject { put("on", follow) }) }
        if (follow) {
            app.connection.events.collect { ev ->
                if (ev.t == "ev.clip") {
                    val t = ((ev.d as? JsonObject)?.get("text") as? JsonPrimitive)?.content ?: return@collect
                    cm.setPrimaryClip(ClipData.newPlainText("PC", t))
                    status = "> received ${t.length} chars from the PC"
                }
            }
        }
    }
    DisposableEffect(Unit) {
        onDispose { app.connection.fire("clip.sub", buildJsonObject { put("on", false) }) }
    }

    Column(Modifier.fillMaxSize()) {
        StatusLine(app)
        ScreenHeader("CLIPBOARD", onBack = { nav.pop() })
        Column(Modifier.fillMaxSize().padding(horizontal = 14.dp).verticalScroll(rememberScrollState())) {
            SectionHeader("PHONE → PC")
            TerminalButton("SEND PHONE CLIPBOARD", {
                val t = cm.primaryClip?.getItemAt(0)?.coerceToText(ctx)?.toString().orEmpty()
                if (t.isEmpty()) {
                    status = "> phone clipboard is empty"
                } else {
                    scope.launch {
                        status = runCatching { app.connection.request("clip.set", buildJsonObject { put("text", t) }); "> sent ${t.length} chars; paste with Ctrl+V" }.getOrElse { "> ${it.message}" }
                    }
                }
            }, Modifier.fillMaxWidth(), style = ButtonStyle.PRIMARY)
            Spacer(Modifier.height(8.dp))
            PromptField(text, { text = it }, placeholder = "or type text here", singleLine = false)
            Spacer(Modifier.height(6.dp))
            TerminalButton("SEND TEXT", {
                scope.launch {
                    status = runCatching { app.connection.request("clip.set", buildJsonObject { put("text", text) }); "> sent" }.getOrElse { "> ${it.message}" }
                }
            }, Modifier.fillMaxWidth(), enabled = text.isNotEmpty())
            SectionHeader("PC → PHONE")
            TerminalButton("GET PC CLIPBOARD", {
                scope.launch {
                    status = runCatching {
                        val d = app.connection.request("clip.get") as JsonObject
                        val t = (d["text"] as? JsonPrimitive)?.content.orEmpty()
                        cm.setPrimaryClip(ClipData.newPlainText("PC", t))
                        "> copied ${t.length} chars to the phone"
                    }.getOrElse { "> ${it.message}" }
                }
            }, Modifier.fillMaxWidth())
            Spacer(Modifier.height(8.dp))
            TerminalButton(if (follow) "FOLLOW PC CLIPBOARD :: ON" else "FOLLOW PC CLIPBOARD :: OFF", { follow = !follow }, Modifier.fillMaxWidth(), latched = follow)
            Text("While this screen is open, text copied on the PC lands on the phone.", style = MType.micro, color = c.dim, modifier = Modifier.padding(top = 6.dp))
            status?.let { Text(it, style = MType.mono, color = c.bright, modifier = Modifier.padding(vertical = 12.dp)) }
        }
    }
}
