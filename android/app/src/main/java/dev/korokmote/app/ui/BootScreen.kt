package dev.korokmote.app.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.korokmote.app.KorokApp
import dev.korokmote.app.design.BlinkCursor
import dev.korokmote.app.design.DecodeText
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import kotlinx.coroutines.delay

@Composable
fun BootScreen(app: KorokApp, nav: Nav) {
    val cfg = LocalMatrix.current
    LaunchedEffect(Unit) {
        delay(if (cfg.full) 900L else 0L)
        val last = app.hosts.lastHostId?.let { app.hosts.get(it) }
        nav.replaceAll(Route.Hosts)
        if (last != null) app.connection.connect(last)
    }
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                DecodeText("KOROKMOTE", style = MType.display)
                BlinkCursor(MType.display)
            }
            DecodeText("> remote for your PC", Modifier.padding(top = 8.dp), style = MType.mono, color = cfg.colors.phosphor)
        }
    }
}
