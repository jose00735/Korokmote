package dev.korokmote.app.design

import android.content.Context
import android.os.PowerManager
import android.provider.Settings as SysSettings
import androidx.compose.foundation.shape.CutCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.korokmote.app.R
import dev.korokmote.app.data.Accent
import dev.korokmote.app.data.GlyphSet
import dev.korokmote.app.data.Intensity
import dev.korokmote.app.data.RainDensity
import dev.korokmote.app.data.Settings

/** Colour tokens from docs/DESIGN.md section 1. */
@Immutable
data class MatrixColors(
    val void: Color = Color(0xFF000000),
    val surface: Color = Color(0xFF050A05),
    val raised: Color = Color(0xFF0A140A),
    val dim: Color,
    val phosphor: Color,
    val bright: Color,
    val head: Color,
    val warn: Color = Color(0xFFFFB000),
    val danger: Color = Color(0xFFFF2A2A),
)

fun paletteFor(accent: Accent): MatrixColors = when (accent) {
    Accent.GREEN -> MatrixColors(dim = Color(0xFF003B00), phosphor = Color(0xFF008F11), bright = Color(0xFF00FF41), head = Color(0xFFD5FFD9))
    Accent.AMBER -> MatrixColors(dim = Color(0xFF3B2A00), phosphor = Color(0xFF8F6A00), bright = Color(0xFFFFB000), head = Color(0xFFFFF0C8))
    Accent.CYAN -> MatrixColors(dim = Color(0xFF00303B), phosphor = Color(0xFF00788F), bright = Color(0xFF00E5FF), head = Color(0xFFD0FAFF))
    Accent.RED -> MatrixColors(dim = Color(0xFF3B0000), phosphor = Color(0xFF8F1111), bright = Color(0xFFFF4141), head = Color(0xFFFFD6D6))
}

object Glyphs {
    const val KATAKANA = "ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉﾊﾋﾌﾍﾎﾏﾐﾑﾒﾓﾔﾕﾖﾗﾘﾙﾚﾛﾜﾝ012345789Z:.=*+-<>¦"
    const val BINARY = "01"
    const val HEX = "0123456789ABCDEF"
    fun of(set: GlyphSet) = when (set) {
        GlyphSet.KATAKANA -> KATAKANA
        GlyphSet.BINARY -> BINARY
        GlyphSet.HEX -> HEX
    }
}

@Immutable
data class MatrixConfig(
    val colors: MatrixColors,
    val intensity: Intensity,
    val density: RainDensity,
    val glyphs: String,
    val haptics: Int,
    val flicker: Boolean,
) {
    val full get() = intensity == Intensity.FULL
    val animated get() = intensity != Intensity.OFF

    /** Scales a duration by the intensity setting. */
    fun dur(ms: Int): Int = when (intensity) {
        Intensity.FULL -> ms
        Intensity.REDUCED -> (ms / 3).coerceAtLeast(1)
        Intensity.OFF -> 0
    }
}

val LocalMatrix = staticCompositionLocalOf {
    MatrixConfig(paletteFor(Accent.GREEN), Intensity.FULL, RainDensity.NORMAL, Glyphs.KATAKANA, 2, false)
}

val Mono = FontFamily(
    Font(R.font.jetbrains_mono_regular, FontWeight.Normal),
    Font(R.font.jetbrains_mono_medium, FontWeight.Medium),
    Font(R.font.jetbrains_mono_bold, FontWeight.Bold),
)

/** Type scale from docs/DESIGN.md section 2. */
object MType {
    val display = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Bold, fontSize = 28.sp, letterSpacing = 2.sp)
    val title = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Medium, fontSize = 18.sp, letterSpacing = 1.sp)
    val body = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Normal, fontSize = 15.sp)
    val mono = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Normal, fontSize = 13.sp)
    val micro = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Normal, fontSize = 11.sp, letterSpacing = 1.sp)
    val button = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Medium, fontSize = 14.sp, letterSpacing = 1.sp)
}

val CutShape = CutCornerShape(topStart = 6.dp, topEnd = 0.dp, bottomEnd = 6.dp, bottomStart = 0.dp)
val CutShapeSmall = CutCornerShape(topStart = 4.dp, topEnd = 0.dp, bottomEnd = 4.dp, bottomStart = 0.dp)

/** Effective intensity: the system "remove animations" switch forces OFF; battery saver caps at REDUCED. */
fun effectiveIntensity(ctx: Context, wanted: Intensity): Intensity {
    val scale = runCatching { SysSettings.Global.getFloat(ctx.contentResolver, SysSettings.Global.ANIMATOR_DURATION_SCALE, 1f) }.getOrDefault(1f)
    if (scale == 0f) return Intensity.OFF
    val saver = (ctx.getSystemService(Context.POWER_SERVICE) as PowerManager).isPowerSaveMode
    if (saver && wanted == Intensity.FULL) return Intensity.REDUCED
    return wanted
}

@Composable
fun MatrixTheme(settings: Settings, content: @Composable () -> Unit) {
    val ctx = LocalContext.current
    val cfg = remember(settings) {
        MatrixConfig(
            colors = paletteFor(settings.accent),
            intensity = effectiveIntensity(ctx, settings.intensity),
            density = settings.density,
            glyphs = Glyphs.of(settings.glyphs),
            haptics = settings.haptics,
            flicker = settings.crtFlicker,
        )
    }
    val c = cfg.colors
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = c.bright, onPrimary = c.void, secondary = c.phosphor, background = c.void,
            surface = c.surface, onSurface = c.bright, onBackground = c.bright, error = c.danger,
            outline = c.dim, surfaceVariant = c.raised, onSurfaceVariant = c.phosphor,
        ),
    ) {
        CompositionLocalProvider(LocalMatrix provides cfg, content = content)
    }
}
