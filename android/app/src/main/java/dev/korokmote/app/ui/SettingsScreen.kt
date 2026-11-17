package dev.korokmote.app.ui

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
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.korokmote.app.BuildConfig
import dev.korokmote.app.KorokApp
import dev.korokmote.app.data.Accent
import dev.korokmote.app.data.GlyphSet
import dev.korokmote.app.data.Intensity
import dev.korokmote.app.data.RainDensity
import dev.korokmote.app.design.GlyphSlider
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.SectionHeader
import dev.korokmote.app.design.TerminalButton
import kotlin.math.roundToInt

@Composable
private fun <T> Choice(label: String, options: List<T>, selected: T, name: (T) -> String, onPick: (T) -> Unit) {
    Text(label, style = MType.micro, color = LocalMatrix.current.colors.phosphor, modifier = Modifier.padding(top = 8.dp, bottom = 4.dp))
    Row(Modifier.fillMaxWidth()) {
        options.forEach { o ->
            TerminalButton(name(o), { onPick(o) }, Modifier.weight(1f).padding(horizontal = 2.dp), latched = o == selected, minHeight = 44.dp, textStyle = MType.micro)
        }
    }
}

@Composable
private fun Toggle(label: String, value: Boolean, onChange: (Boolean) -> Unit) {
    TerminalButton((if (value) "[x] " else "[ ] ") + label, { onChange(!value) }, Modifier.fillMaxWidth().padding(vertical = 3.dp), minHeight = 48.dp, textStyle = MType.mono)
}

@Composable
fun SettingsScreen(app: KorokApp, nav: Nav) {
    val s by app.settings.state.collectAsState()
    val c = LocalMatrix.current.colors
    val up = app.settings::update
    Column(Modifier.fillMaxSize()) {
        ScreenHeader("SETTINGS", onBack = { nav.pop() })
        Column(Modifier.fillMaxSize().padding(horizontal = 14.dp).verticalScroll(rememberScrollState())) {
            SectionHeader("POINTER")
            GlyphSlider((s.pointerSpeed - 0.3f) / 2.7f, { v -> up { it.copy(pointerSpeed = 0.3f + v * 2.7f) } }, label = "SPEED")
            GlyphSlider(s.acceleration / 2f, { v -> up { it.copy(acceleration = v * 2f) } }, label = "ACCELERATION")
            GlyphSlider((s.scrollSpeed - 0.2f) / 2.8f, { v -> up { it.copy(scrollSpeed = 0.2f + v * 2.8f) } }, label = "SCROLL SPEED")
            Toggle("Invert scroll direction", s.naturalScroll) { v -> up { it.copy(naturalScroll = v) } }
            Toggle("Tap to click", s.tapToClick) { v -> up { it.copy(tapToClick = v) } }
            SectionHeader("FEEL")
            Choice("HAPTICS", listOf(0, 1, 2), s.haptics, { listOf("OFF", "LIGHT", "NORMAL")[it] }) { v -> up { it.copy(haptics = v) } }
            Toggle("Volume keys control the PC", s.volumeKeys) { v -> up { it.copy(volumeKeys = v) } }
            Toggle("Keep screen on while controlling", s.keepScreenOn) { v -> up { it.copy(keepScreenOn = v) } }
            GlyphSlider(s.backgroundMinutes / 30f, { v -> up { it.copy(backgroundMinutes = (v * 30).roundToInt()) } }, label = "DISCONNECT IN BACKGROUND AFTER ${s.backgroundMinutes} MIN (0 = NEVER)", steps = 30)
            SectionHeader("LOOK")
            Choice("ANIMATION", Intensity.entries, s.intensity, { it.name }) { v -> up { it.copy(intensity = v) } }
            Choice("RAIN DENSITY", RainDensity.entries, s.density, { it.name }) { v -> up { it.copy(density = v) } }
            Choice("GLYPHS", GlyphSet.entries, s.glyphs, { it.name }) { v -> up { it.copy(glyphs = v) } }
            Choice("ACCENT", Accent.entries, s.accent, { it.name }) { v -> up { it.copy(accent = v) } }
            Toggle("CRT flicker", s.crtFlicker) { v -> up { it.copy(crtFlicker = v) } }
            SectionHeader("ABOUT")
            Text(
                "Korokmote ${BuildConfig.VERSION_NAME}\nOpen source, MIT licence.\nNo accounts, no cloud: your phone talks only to your PC.\n\nJetBrains Mono font: SIL Open Font License 1.1.\nhttps://github.com/jose00735/Korokmote\n\nDevice id: ${app.hosts.deviceId.take(8)}…",
                style = MType.mono, color = c.phosphor,
            )
            Spacer(Modifier.height(24.dp))
        }
    }
}
