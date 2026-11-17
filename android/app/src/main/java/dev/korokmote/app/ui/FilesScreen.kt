package dev.korokmote.app.ui

import androidx.compose.foundation.horizontalScroll
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
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.ButtonStyle
import dev.korokmote.app.design.BracketAction
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.HexDialog
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.PromptField
import dev.korokmote.app.design.TerminalButton
import dev.korokmote.app.design.TerminalRow
import dev.korokmote.app.protocol.AppInfo
import dev.korokmote.app.protocol.FsEntry
import dev.korokmote.app.protocol.Protocol
import dev.korokmote.app.protocol.WindowInfo
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonObject

@Composable
fun FilesScreen(app: KorokApp, nav: Nav, initialTab: Int) {
    var tab by remember { mutableIntStateOf(initialTab) }
    Column(Modifier.fillMaxSize()) {
        StatusLine(app)
        ScreenHeader(listOf("FILES", "APPS", "WINDOWS")[tab], onBack = { nav.pop() })
        Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp)) {
            listOf("FILES", "APPS", "WINDOWS").forEachIndexed { i, t ->
                TerminalButton(t, { tab = i }, Modifier.weight(1f).padding(horizontal = 3.dp), latched = tab == i, minHeight = 44.dp)
            }
        }
        when (tab) {
            0 -> FileBrowser(app)
            1 -> AppLauncher(app)
            else -> WindowSwitcher(app)
        }
    }
}

private fun human(n: Long): String = when {
    n >= 1L shl 30 -> "%.1f GB".format(n / (1L shl 30).toDouble())
    n >= 1L shl 20 -> "%.1f MB".format(n / (1L shl 20).toDouble())
    n >= 1L shl 10 -> "%.0f KB".format(n / (1L shl 10).toDouble())
    else -> "$n B"
}

@Composable
private fun FileBrowser(app: KorokApp) {
    val c = LocalMatrix.current.colors
    val scope = rememberCoroutineScope()
    var stack by remember { mutableStateOf(listOf<FsEntry>()) }
    var entries by remember { mutableStateOf<List<FsEntry>>(emptyList()) }
    var error by remember { mutableStateOf<String?>(null) }
    var confirm by remember { mutableStateOf<FsEntry?>(null) }
    suspend fun load() {
        try {
            val d = if (stack.isEmpty()) app.connection.request("fs.roots") else app.connection.request("fs.list", Msg.path(stack.last().path))
            entries = Protocol.json.decodeFromJsonElement(ListSerializer(FsEntry.serializer()), (d as JsonObject)["entries"]!!)
            error = null
        } catch (e: Exception) {
            error = e.message
        }
    }
    LaunchedEffect(stack) { load() }
    Column(Modifier.fillMaxSize().padding(horizontal = 12.dp)) {
        Row(Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())) {
            BracketAction("ROOT", { stack = emptyList() })
            stack.forEachIndexed { i, e ->
                Text("/", style = MType.mono, color = c.dim, modifier = Modifier.padding(top = 10.dp))
                BracketAction(e.name.take(18), { stack = stack.take(i + 1) })
            }
        }
        error?.let { Text("> $it", style = MType.mono, color = c.danger) }
        LazyColumn(Modifier.weight(1f)) {
            itemsIndexed(entries, key = { _, e -> e.path }) { i, e ->
                TerminalRow(
                    i, (if (e.dir) "▸ " else "  ") + e.name,
                    subtitle = if (e.dir) null else human(e.size),
                    titleColor = if (e.dir) c.bright else c.phosphor,
                ) { if (e.dir) stack = stack + e else confirm = e }
            }
        }
    }
    confirm?.let { e ->
        HexDialog("OPEN ON PC", onDismiss = { confirm = null }) {
            Text(e.name, style = MType.body, color = c.bright)
            Text(human(e.size), style = MType.mono, color = c.phosphor)
            Spacer(Modifier.height(14.dp))
            Row {
                TerminalButton("OPEN", {
                    confirm = null
                    scope.launch { runCatching { app.connection.request("fs.open", Msg.path(e.path)) }.onFailure { error = it.message } }
                }, Modifier.weight(1f), style = ButtonStyle.PRIMARY)
                Spacer(Modifier.width(8.dp))
                TerminalButton("CANCEL", { confirm = null }, Modifier.weight(1f))
            }
        }
    }
}

@Composable
private fun AppLauncher(app: KorokApp) {
    val c = LocalMatrix.current.colors
    var apps by remember { mutableStateOf<List<AppInfo>>(emptyList()) }
    var filter by remember { mutableStateOf("") }
    var status by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) {
        runCatching {
            val d = app.connection.request("apps.list", timeoutMs = 15000) as JsonObject
            apps = Protocol.json.decodeFromJsonElement(ListSerializer(AppInfo.serializer()), d["apps"]!!)
        }.onFailure { status = "> ${it.message}" }
    }
    Column(Modifier.fillMaxSize().padding(horizontal = 12.dp)) {
        Spacer(Modifier.height(8.dp))
        PromptField(filter, { filter = it }, placeholder = "filter apps")
        status?.let { Text(it, style = MType.mono, color = c.bright, modifier = Modifier.padding(vertical = 6.dp)) }
        if (apps.isEmpty() && status == null) DecodeText("> reading start menu…", style = MType.mono, color = c.phosphor)
        val shown = apps.filter { filter.isBlank() || it.name.contains(filter, ignoreCase = true) }
        LazyColumn(Modifier.weight(1f)) {
            itemsIndexed(shown, key = { _, a -> a.id }) { i, a ->
                TerminalRow(i, a.name) {
                    scope.launch {
                        status = runCatching { app.connection.request("apps.launch", Msg.id(a.id)); "> launched ${a.name}" }.getOrElse { "> ${it.message}" }
                    }
                }
            }
        }
    }
}

@Composable
private fun WindowSwitcher(app: KorokApp) {
    val c = LocalMatrix.current.colors
    var windows by remember { mutableStateOf<List<WindowInfo>>(emptyList()) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    suspend fun load() {
        runCatching {
            val d = app.connection.request("win.list") as JsonObject
            windows = Protocol.json.decodeFromJsonElement(ListSerializer(WindowInfo.serializer()), d["windows"]!!)
        }.onFailure { error = it.message }
    }
    LaunchedEffect(Unit) { load() }
    Column(Modifier.fillMaxSize().padding(horizontal = 12.dp)) {
        Row {
            Text("tap :: focus   hold :: close", style = MType.micro, color = c.phosphor, modifier = Modifier.weight(1f).padding(top = 12.dp))
            BracketAction("REFRESH", { scope.launch { load() } })
        }
        error?.let { Text("> $it", style = MType.mono, color = c.danger) }
        LazyColumn(Modifier.weight(1f)) {
            itemsIndexed(windows, key = { _, w -> w.h }) { i, w ->
                TerminalRow(i, w.title, subtitle = w.exe, onLongClick = {
                    scope.launch {
                        runCatching { app.connection.request("win.close", Msg.h(w.h)) }
                        load()
                    }
                }) { app.connection.fire("win.focus", Msg.h(w.h)) }
            }
        }
    }
}
