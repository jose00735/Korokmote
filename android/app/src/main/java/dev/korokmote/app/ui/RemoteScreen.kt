package dev.korokmote.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
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
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.GlyphSlider
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.PromptField
import dev.korokmote.app.design.TerminalButton
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.floatOrNull
import kotlinx.serialization.json.intOrNull
import kotlin.math.roundToInt

private val icons = mapOf(
    "play_pause" to "▶ ❚❚", "play" to "▶", "pause" to "❚❚", "stop" to "■", "prev" to "|◀◀", "next" to "▶▶|",
    "back" to "←", "forward" to "→", "refresh" to "↻", "up" to "↑", "down" to "↓", "left" to "←", "right" to "→",
    "mute" to "∅", "plus" to "+", "minus" to "-", "power" to "⏻",
)

/** Renders a daemon-served remote layout (REMOTES.md). Unknown widgets are skipped. */
@Composable
fun RemoteScreen(app: KorokApp, nav: Nav, id: String) {
    val c = LocalMatrix.current.colors
    var doc by remember { mutableStateOf<JsonObject?>(null) }
    var error by remember { mutableStateOf<String?>(null) }
    suspend fun load() {
        try {
            doc = app.connection.request("remotes.get", Msg.id(id)) as JsonObject
            error = null
        } catch (e: Exception) {
            error = e.message
        }
    }
    LaunchedEffect(id) {
        load()
        app.connection.events.collect { if (it.t == "ev.remotes") load() }
    }
    val name = (doc?.get("name") as? JsonPrimitive)?.content ?: id
    val act: (String, Map<String, String>) -> Unit = { action, args -> app.connection.fire("remotes.act", Msg.act(id, action, args)) }
    Column(Modifier.fillMaxSize()) {
        StatusLine(app)
        ScreenHeader(name.uppercase(), onBack = { nav.pop() })
        Column(Modifier.fillMaxSize().padding(horizontal = 12.dp).verticalScroll(rememberScrollState())) {
            error?.let { Text("> $it", style = MType.mono, color = c.danger) }
            (doc?.get("layout") as? JsonArray)?.forEach { Widget(app, it, act, Modifier.fillMaxWidth()) }
            Spacer(Modifier.height(20.dp))
        }
    }
}

private fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.content
private fun JsonObject.i(k: String) = (this[k] as? JsonPrimitive)?.intOrNull
private fun JsonObject.f(k: String) = (this[k] as? JsonPrimitive)?.floatOrNull
private fun JsonObject.b(k: String) = (this[k] as? JsonPrimitive)?.booleanOrNull

@Composable
private fun Widget(app: KorokApp, el: JsonElement, act: (String, Map<String, String>) -> Unit, modifier: Modifier) {
    val obj = el as? JsonObject ?: return
    val (type, body) = obj.entries.firstOrNull() ?: return
    val c = LocalMatrix.current.colors
    when (type) {
        "row" -> Row(modifier.padding(vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            (body as? JsonArray)?.forEach { child -> RowChild(app, child, act) }
        }
        "grid" -> {
            val g = body as? JsonObject ?: return
            val cols = (g.i("columns") ?: 3).coerceIn(1, 8)
            val items = (g["items"] as? JsonArray).orEmpty()
            Column(modifier.padding(vertical = 4.dp)) {
                items.chunked(cols).forEach { row ->
                    Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        row.forEach { child -> Box(Modifier.weight(1f)) { Widget(app, child, act, Modifier.fillMaxWidth()) } }
                        repeat(cols - row.size) { Spacer(Modifier.weight(1f)) }
                    }
                }
            }
        }
        "button" -> {
            val b = body as? JsonObject ?: return
            val action = b.s("action") ?: return
            val label = b.s("label") ?: icons[b.s("icon")] ?: action
            val style = when (b.s("style")) {
                "primary" -> ButtonStyle.PRIMARY
                "danger" -> ButtonStyle.DANGER
                else -> ButtonStyle.DEFAULT
            }
            TerminalButton(
                label, { act(action, emptyMap()) }, modifier.padding(vertical = 2.dp), style = style,
                repeat = b.b("repeat") == true,
                onLongClick = b.s("hold")?.let { h -> { act(h, emptyMap()) } },
                description = b.s("label") ?: b.s("icon")?.replace('_', ' ') ?: action,
            )
        }
        "toggle" -> {
            val t = body as? JsonObject ?: return
            var on by remember { mutableStateOf(false) }
            TerminalButton((t.s("label") ?: "TOGGLE") + if (on) " :: ON" else " :: OFF", {
                on = !on
                (if (on) t.s("on") else t.s("off"))?.let { act(it, emptyMap()) }
            }, modifier.padding(vertical = 2.dp), latched = on)
        }
        "slider" -> {
            val s = body as? JsonObject ?: return
            val steps = (s.i("steps") ?: 20).coerceIn(2, 100)
            var value by remember { mutableFloatStateOf(0.5f) }
            GlyphSlider(value, { v ->
                val from = (value * steps).roundToInt()
                val to = (v * steps).roundToInt()
                value = v
                val set = s.s("set")
                when {
                    set != null -> if (from != to) act(set, mapOf("value" to "%.2f".format(java.util.Locale.ROOT, v)))
                    to > from -> repeat(to - from) { s.s("up")?.let { act(it, emptyMap()) } }
                    to < from -> repeat(from - to) { s.s("down")?.let { act(it, emptyMap()) } }
                }
            }, modifier.padding(vertical = 6.dp), steps = steps, label = s.s("label") ?: "LEVEL")
        }
        "dpad" -> {
            val d = body as? JsonObject ?: return
            Column(modifier.padding(vertical = 6.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                d.s("up")?.let { TerminalButton("↑", { act(it, emptyMap()) }, Modifier.width(84.dp), repeat = true) }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    d.s("left")?.let { TerminalButton("←", { act(it, emptyMap()) }, Modifier.width(84.dp), repeat = true) }
                    Spacer(Modifier.width(6.dp))
                    d.s("center")?.let { TerminalButton("OK", { act(it, emptyMap()) }, Modifier.width(84.dp), style = ButtonStyle.PRIMARY) } ?: Spacer(Modifier.width(84.dp))
                    Spacer(Modifier.width(6.dp))
                    d.s("right")?.let { TerminalButton("→", { act(it, emptyMap()) }, Modifier.width(84.dp), repeat = true) }
                }
                d.s("down")?.let { TerminalButton("↓", { act(it, emptyMap()) }, Modifier.width(84.dp), repeat = true) }
            }
        }
        "touchpad" -> {
            val t = body as? JsonObject
            val settings by app.settings.state.collectAsState()
            TouchpadSurface(app.connection, settings, modifier.padding(vertical = 6.dp).height(((t?.i("height") ?: 3).coerceIn(1, 8) * 64).dp), hint = false)
        }
        "input" -> {
            val f = body as? JsonObject ?: return
            val action = f.s("action") ?: return
            var text by remember { mutableStateOf("") }
            Column(modifier.padding(vertical = 4.dp)) {
                f.s("label")?.let { Text(it, style = MType.micro, color = c.phosphor) }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    PromptField(text, { text = it }, Modifier.weight(1f), placeholder = f.s("placeholder") ?: "", onDone = {
                        if (text.isNotBlank()) act(action, mapOf("query" to text))
                    })
                    Spacer(Modifier.width(8.dp))
                    TerminalButton("GO", { if (text.isNotBlank()) act(action, mapOf("query" to text)) }, Modifier.width(64.dp), style = ButtonStyle.PRIMARY)
                }
            }
        }
        "label" -> {
            val l = body as? JsonObject
            DecodeText(l?.s("text") ?: "", modifier.padding(vertical = 6.dp), style = MType.mono, color = c.phosphor)
        }
        "spacer" -> Spacer(Modifier.height((((body as? JsonObject)?.i("size") ?: 1) * 12).dp))
        else -> Unit
    }
}

@Composable
private fun RowScope.RowChild(app: KorokApp, child: JsonElement, act: (String, Map<String, String>) -> Unit) {
    val weight = ((child as? JsonObject)?.values?.firstOrNull() as? JsonObject)?.f("size") ?: 1f
    Box(Modifier.weight(weight.coerceIn(0.2f, 6f))) { Widget(app, child, act, Modifier.fillMaxWidth()) }
}
