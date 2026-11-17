package dev.korokmote.app.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.ButtonRow
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.KeyCap
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.PromptField
import dev.korokmote.app.design.SectionHeader
import dev.korokmote.app.design.TerminalButton
import dev.korokmote.app.design.TerminalRow
import dev.korokmote.app.design.VolumeRing
import dev.korokmote.app.protocol.AudioDevice
import dev.korokmote.app.protocol.Keys
import dev.korokmote.app.protocol.Protocol
import dev.korokmote.app.protocol.RemoteSummary
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonObject

@Composable
fun MainScreen(app: KorokApp, nav: Nav, tab: Tab) {
    Column(Modifier.fillMaxSize()) {
        StatusLine(app)
        Box(Modifier.weight(1f).fillMaxWidth()) {
            when (tab) {
                Tab.PAD -> TouchpadTab(app, nav)
                Tab.KEYS -> KeyboardTab(app)
                Tab.MEDIA -> MediaTab(app)
                Tab.REMOTES -> RemotesTab(app, nav)
                Tab.MORE -> MoreTab(app, nav)
            }
        }
        CommandBar(tab) { nav.replaceTop(Route.Main(it)) }
    }
}

@Composable
private fun TouchpadTab(app: KorokApp, nav: Nav) {
    val settings by app.settings.state.collectAsState()
    val conn = app.connection
    Column(Modifier.fillMaxSize().padding(10.dp)) {
        TouchpadSurface(conn, settings, Modifier.weight(1f).fillMaxWidth())
        Spacer(Modifier.height(8.dp))
        Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            listOf("ESC", "TAB", "WIN", "LEFT", "UP", "DOWN", "RIGHT", "ENTER", "BACKSPACE").forEach { k ->
                KeyCap(Keys.label(k), { conn.send("key", Msg.key(k)) }, Modifier.width(58.dp), repeat = k in Keys.arrows || k == "BACKSPACE")
            }
            KeyCap("ALT+TAB", { conn.send("combo", Msg.combo(listOf("ALT", "TAB"))) }, Modifier.width(84.dp))
            KeyCap("⌨", { nav.replaceTop(Route.Main(Tab.KEYS)) }, Modifier.width(58.dp))
        }
    }
}

/**
 * Keyboard: live typing through a hidden-buffer text field, modifier latches and special keys.
 * In live mode each character is sent as typed; in compose mode the text is sent on [ SEND ].
 */
@Composable
private fun KeyboardTab(app: KorokApp) {
    val c = LocalMatrix.current.colors
    val conn = app.connection
    var live by remember { mutableStateOf(true) }
    val sentinel = "      "
    var field by remember { mutableStateOf(TextFieldValue(sentinel, TextRange(sentinel.length))) }
    var compose by remember { mutableStateOf("") }
    val latched = remember { mutableStateListOf<String>() }
    val locked = remember { mutableStateListOf<String>() }
    val focus = remember { FocusRequester() }
    val kb = LocalSoftwareKeyboardController.current
    LaunchedEffect(live) {
        if (live) {
            focus.requestFocus()
            kb?.show()
        }
    }

    fun press(k: String) {
        val mods = (locked + latched).distinct()
        if (mods.isEmpty()) {
            conn.send("key", Msg.key(k))
        } else {
            conn.send("combo", Msg.combo(mods + k))
            latched.clear()
        }
    }

    fun toggleMod(m: String) {
        when {
            locked.contains(m) -> locked.remove(m)
            latched.contains(m) -> {
                latched.remove(m)
                locked.add(m)
            }
            else -> latched.add(m)
        }
    }

    Column(Modifier.fillMaxSize().padding(10.dp).verticalScroll(rememberScrollState())) {
        Row {
            TerminalButton("LIVE", { live = true }, Modifier.weight(1f), latched = live, minHeight = 44.dp)
            Spacer(Modifier.width(8.dp))
            TerminalButton("COMPOSE", { live = false }, Modifier.weight(1f), latched = !live, minHeight = 44.dp)
        }
        Spacer(Modifier.height(8.dp))
        if (live) {
            Text("> type on your phone keyboard; it goes straight to the PC", style = MType.micro, color = c.phosphor)
            BasicTextField(
                value = field,
                onValueChange = { v ->
                    val old = field.text
                    val new = v.text
                    when {
                        new.length > old.length && new.startsWith(old) -> {
                            val added = new.substring(old.length)
                            val mods = (locked + latched).distinct()
                            if (mods.isNotEmpty() && added.length == 1 && added[0].isLetterOrDigit()) {
                                conn.send("combo", Msg.combo(mods + added.uppercase()))
                                latched.clear()
                            } else {
                                conn.send("text", Msg.text(added))
                            }
                        }
                        new.length < old.length -> repeat(old.length - new.length) { conn.send("key", Msg.key("BACKSPACE")) }
                        new != old -> {
                            // Autocorrect replaced a word: erase the difference and retype.
                            val common = old.commonPrefixWith(new).length
                            repeat(old.length - common) { conn.send("key", Msg.key("BACKSPACE")) }
                            if (new.length > common) conn.send("text", Msg.text(new.substring(common)))
                        }
                    }
                    field = if (v.text.length > 64 || v.text.length < sentinel.length) {
                        TextFieldValue(sentinel, TextRange(sentinel.length))
                    } else {
                        v.copy(selection = TextRange(v.text.length))
                    }
                },
                textStyle = MType.body.copy(color = c.bright),
                cursorBrush = SolidColor(c.bright),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Text, imeAction = ImeAction.Send, autoCorrectEnabled = false),
                keyboardActions = KeyboardActions(onSend = { conn.send("key", Msg.key("ENTER")) }),
                modifier = Modifier.fillMaxWidth().padding(vertical = 10.dp).focusRequester(focus),
            )
        } else {
            PromptField(compose, { compose = it }, placeholder = "text to send", singleLine = false, imeAction = ImeAction.Default)
            Spacer(Modifier.height(6.dp))
            ButtonRow {
                TerminalButton("SEND", { if (compose.isNotEmpty()) { conn.send("text", Msg.text(compose)); compose = "" } }, Modifier.weight(1f), style = ButtonStyle.PRIMARY)
                TerminalButton("SEND + ENTER", {
                    conn.send("text", Msg.text(compose))
                    conn.send("key", Msg.key("ENTER"))
                    compose = ""
                }, Modifier.weight(1f))
            }
        }
        SectionHeader("MODIFIERS")
        ButtonRow {
            Keys.modifiers.forEach { m ->
                KeyCap(m, { toggleMod(m) }, Modifier.weight(1f), latched = latched.contains(m), locked = locked.contains(m))
            }
        }
        Text("tap = next key only :: tap twice = lock", style = MType.micro, color = c.dim)
        SectionHeader("KEYS")
        Keys.navigation.chunked(5).forEach { row ->
            ButtonRow { row.forEach { k -> KeyCap(Keys.label(k), { press(k) }, Modifier.weight(1f), repeat = k == "BACKSPACE" || k == "DELETE") } }
        }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
            Column(horizontalAlignment = Alignment.CenterHorizontally) {
                KeyCap("↑", { press("UP") }, Modifier.width(72.dp), repeat = true)
                Row {
                    KeyCap("←", { press("LEFT") }, Modifier.width(72.dp), repeat = true)
                    Spacer(Modifier.width(6.dp))
                    KeyCap("↓", { press("DOWN") }, Modifier.width(72.dp), repeat = true)
                    Spacer(Modifier.width(6.dp))
                    KeyCap("→", { press("RIGHT") }, Modifier.width(72.dp), repeat = true)
                }
            }
        }
        SectionHeader("FUNCTION")
        Keys.function.chunked(6).forEach { row ->
            ButtonRow { row.forEach { k -> KeyCap(k, { press(k) }, Modifier.weight(1f)) } }
        }
        ButtonRow { Keys.system.forEach { k -> KeyCap(k, { press(k) }, Modifier.weight(1f)) } }
        SectionHeader("SHORTCUTS")
        Keys.shortcuts.chunked(3).forEach { row ->
            ButtonRow {
                row.forEach { s -> TerminalButton(s.label, { conn.send("combo", Msg.combo(s.keys)) }, Modifier.weight(1f), minHeight = 48.dp, textStyle = MType.mono) }
                repeat(3 - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
        Spacer(Modifier.height(16.dp))
    }
}

@Composable
private fun MediaTab(app: KorokApp) {
    val c = LocalMatrix.current.colors
    val conn = app.connection
    val audio by conn.audio.collectAsState()
    val scope = rememberCoroutineScope()
    var devices by remember { mutableStateOf<List<AudioDevice>>(emptyList()) }
    var selected by remember { mutableStateOf("default") }
    var local by remember { mutableStateOf<Float?>(null) }
    suspend fun loadDevices() {
        runCatching {
            val d = conn.request("audio.list") as JsonObject
            devices = Protocol.json.decodeFromJsonElement(ListSerializer(AudioDevice.serializer()), d["devices"]!!)
        }
    }
    LaunchedEffect(Unit) { loadDevices() }
    val dev = devices.firstOrNull { it.id == selected }
    val level = local ?: if (selected == "default") (audio?.level ?: 0.0).toFloat() else (dev?.level ?: 0.0).toFloat()
    val muted = if (selected == "default") audio?.muted ?: false else dev?.muted ?: false
    Column(Modifier.fillMaxSize().padding(14.dp).verticalScroll(rememberScrollState()), horizontalAlignment = Alignment.CenterHorizontally) {
        VolumeRing(level, muted, onSet = { v ->
            local = v
            scope.launch {
                runCatching { conn.request("audio.set", Msg.volumeSet(v.toDouble(), selected)) }
                local = null
                if (selected != "default") loadDevices()
            }
        })
        ButtonRow {
            TerminalButton("VOL -", { conn.send("audio.set", Msg.volumeDelta(-0.04, selected)) }, Modifier.weight(1f), repeat = true)
            TerminalButton(if (muted) "UNMUTE" else "MUTE", {
                conn.send("audio.mute", kotlinx.serialization.json.buildJsonObject { put("device", kotlinx.serialization.json.JsonPrimitive(selected)) })
            }, Modifier.weight(1f))
            TerminalButton("VOL +", { conn.send("audio.set", Msg.volumeDelta(0.04, selected)) }, Modifier.weight(1f), repeat = true)
        }
        SectionHeader("TRANSPORT")
        ButtonRow {
            TerminalButton("|◀◀", { conn.send("media", Msg.media("prev")) }, Modifier.weight(1f), description = "Previous")
            TerminalButton("▶ ❚❚", { conn.send("media", Msg.media("playpause")) }, Modifier.weight(1.6f), style = ButtonStyle.PRIMARY, description = "Play or pause")
            TerminalButton("▶▶|", { conn.send("media", Msg.media("next")) }, Modifier.weight(1f), description = "Next")
        }
        ButtonRow {
            TerminalButton("■ STOP", { conn.send("media", Msg.media("stop")) }, Modifier.weight(1f))
        }
        if (devices.size > 1) {
            SectionHeader("OUTPUT")
            Column(Modifier.fillMaxWidth()) {
                TerminalRow(0, "Default output", titleColor = if (selected == "default") c.bright else c.phosphor) { selected = "default" }
                devices.forEachIndexed { i, d ->
                    TerminalRow(
                        i + 1, d.name,
                        subtitle = "${(d.level * 100).toInt()}%" + (if (d.muted) " :: muted" else "") + if (d.default) " :: default" else "",
                        titleColor = if (selected == d.id) c.bright else c.phosphor,
                    ) { selected = d.id }
                }
            }
        }
    }
}

@Composable
private fun RemotesTab(app: KorokApp, nav: Nav) {
    val c = LocalMatrix.current.colors
    val conn = app.connection
    val fg by conn.foreground.collectAsState()
    var remotes by remember { mutableStateOf<List<RemoteSummary>>(emptyList()) }
    var error by remember { mutableStateOf<String?>(null) }
    suspend fun load() {
        try {
            val d = conn.request("remotes.list") as JsonObject
            remotes = Protocol.json.decodeFromJsonElement(ListSerializer(RemoteSummary.serializer()), d["remotes"]!!)
            error = null
        } catch (e: Exception) {
            error = e.message
        }
    }
    LaunchedEffect(Unit) {
        load()
        conn.events.collect { if (it.t == "ev.remotes") load() }
    }
    LazyColumn(Modifier.fillMaxSize().padding(horizontal = 14.dp)) {
        val suggested = fg?.remote?.let { id -> remotes.firstOrNull { it.id == id } }
        if (suggested != null) {
            item {
                SectionHeader("DETECTED")
                Text("> ${fg?.exe} is in front", style = MType.mono, color = c.phosphor)
                Spacer(Modifier.height(8.dp))
                TerminalButton("OPEN ${suggested.name.uppercase()} REMOTE", { nav.push(Route.Remote(suggested.id)) }, Modifier.fillMaxWidth(), style = ButtonStyle.PRIMARY)
            }
        }
        item { SectionHeader("REMOTES") }
        itemsIndexed(remotes, key = { _, r -> r.id }) { i, r ->
            TerminalRow(i, r.name, subtitle = r.match?.takeIf { it.isNotEmpty() }?.joinToString(", ")) { nav.push(Route.Remote(r.id)) }
        }
        error?.let { item { Text("> $it", style = MType.mono, color = c.danger) } }
        item {
            Spacer(Modifier.height(12.dp))
            Text("Add your own: drop a .yaml file in the PC's remotes folder (tray → Open remotes folder).", style = MType.micro, color = c.dim)
            Spacer(Modifier.height(16.dp))
        }
    }
}

@Composable
private fun MoreTab(app: KorokApp, nav: Nav) {
    val c = LocalMatrix.current.colors
    Column(Modifier.fillMaxSize().padding(horizontal = 14.dp).verticalScroll(rememberScrollState())) {
        SectionHeader("SYSTEM")
        TerminalRow(0, "Power", subtitle = "lock, sleep, restart, shut down") { nav.push(Route.Power) }
        TerminalRow(1, "Files", subtitle = "browse and open files on the PC") { nav.push(Route.Files(0)) }
        TerminalRow(2, "Apps", subtitle = "launch from the Start menu") { nav.push(Route.Files(1)) }
        TerminalRow(3, "Windows", subtitle = "switch or close open windows") { nav.push(Route.Files(2)) }
        TerminalRow(4, "Clipboard", subtitle = "send text between phone and PC") { nav.push(Route.Clipboard) }
        SectionHeader("APP")
        TerminalRow(5, "Switch PC", subtitle = "paired and discovered PCs") { app.connection.disconnect(); nav.replaceAll(Route.Hosts) }
        TerminalRow(6, "Settings", subtitle = "pointer, haptics, animation, colours") { nav.push(Route.Settings) }
        TerminalRow(7, "Disconnect", titleColor = c.danger) { app.connection.disconnect() }
        Spacer(Modifier.height(18.dp))
        DecodeText("> korokmote ${dev.korokmote.app.BuildConfig.VERSION_NAME} :: open source :: MIT", style = MType.micro, color = c.dim)
    }
}
