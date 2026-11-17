package dev.korokmote.app.ui

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.korokmote.app.design.BlinkCursor
import dev.korokmote.app.design.BracketAction
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.net.ConnectionManager
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** Screen title row: `< BACK   TITLE█`. */
@Composable
fun ScreenHeader(title: String, onBack: (() -> Unit)? = null, trailing: @Composable (() -> Unit)? = null) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        if (onBack != null) BracketAction("<", onBack) else Spacer(Modifier.width(8.dp))
        DecodeText(title, style = MType.title, maxLines = 1)
        BlinkCursor(MType.title)
        Spacer(Modifier.weight(1f))
        trailing?.invoke()
    }
}

/** Convenience builders for protocol payloads. */
object Msg {
    fun key(k: String, a: String = "tap") = buildJsonObject { put("k", k); put("a", a) }
    fun combo(keys: List<String>) = buildJsonObject { put("keys", buildJsonArray { keys.forEach { add(kotlinx.serialization.json.JsonPrimitive(it)) } }) }
    fun text(s: String) = buildJsonObject { put("s", s) }
    fun media(a: String) = buildJsonObject { put("a", a) }
    fun power(a: String, delay: Int = 0) = buildJsonObject { put("a", a); if (delay > 0) put("delay", delay) }
    fun volumeDelta(d: Double, device: String = "default") = buildJsonObject { put("device", device); put("delta", d) }
    fun volumeSet(l: Double, device: String = "default") = buildJsonObject { put("device", device); put("level", l) }
    fun path(p: String) = buildJsonObject { put("path", p) }
    fun id(id: String) = buildJsonObject { put("id", id) }
    fun h(h: Long) = buildJsonObject { put("h", h) }
    fun act(remote: String, action: String, args: Map<String, String> = emptyMap()) = buildJsonObject {
        put("remote", remote)
        put("action", action)
        put("args", JsonObject(args.mapValues { kotlinx.serialization.json.JsonPrimitive(it.value) }))
    }
}

fun ConnectionManager.send(t: String, d: JsonElement? = null) = fire(t, d)

@Composable
fun Hint(text: String, modifier: Modifier = Modifier) {
    androidx.compose.material3.Text(text, style = MType.mono, color = LocalMatrix.current.colors.phosphor, modifier = modifier)
}
