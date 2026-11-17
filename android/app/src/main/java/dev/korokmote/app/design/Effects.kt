package dev.korokmote.app.design

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.Box
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.Paint
import androidx.compose.ui.graphics.drawscope.clipRect
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.drawscope.translate
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.style.TextOverflow
import kotlinx.coroutines.delay
import kotlin.random.Random

/**
 * Text that resolves from random glyphs, left to right (DESIGN.md 5.2).
 * Plays only when [text] changes; accessibility always sees the final text.
 */
@Composable
fun DecodeText(
    text: String,
    modifier: Modifier = Modifier,
    style: TextStyle = MType.body,
    color: Color = LocalMatrix.current.colors.bright,
    maxLines: Int = Int.MAX_VALUE,
) {
    val cfg = LocalMatrix.current
    var shown by remember { mutableStateOf(text) }
    LaunchedEffect(text) {
        if (!cfg.animated || text.isEmpty()) {
            shown = text
            return@LaunchedEffect
        }
        val total = if (cfg.full) minOf(450, 22 * text.length) else 120
        val perChar = total.toFloat() / text.length
        val start = System.nanoTime()
        val rnd = Random(text.hashCode())
        while (true) {
            val elapsed = (System.nanoTime() - start) / 1_000_000f
            val locked = (elapsed / perChar).toInt()
            if (locked >= text.length) break
            shown = buildString(text.length) {
                text.forEachIndexed { i, ch ->
                    append(if (i < locked || ch == ' ') ch else cfg.glyphs[rnd.nextInt(cfg.glyphs.length)])
                }
            }
            delay(40)
        }
        shown = text
    }
    Text(
        shown,
        modifier = modifier.clearAndSetSemantics { contentDescription = text },
        style = style,
        color = color,
        maxLines = maxLines,
        overflow = TextOverflow.Clip,
        softWrap = maxLines != 1,
    )
}

/** Blinking block cursor, 530 ms on / 530 ms off. */
@Composable
fun BlinkCursor(style: TextStyle = MType.body, color: Color = LocalMatrix.current.colors.bright) {
    val cfg = LocalMatrix.current
    var on by remember { mutableStateOf(true) }
    if (cfg.animated) {
        LaunchedEffect(Unit) {
            while (true) {
                delay(530)
                on = !on
            }
        }
    }
    Text(if (on) "█" else " ", style = style, color = color)
}

/** Scanlines and vignette overlay (DESIGN.md 5.4). */
fun Modifier.crt(cfg: MatrixConfig): Modifier = if (!cfg.full) this else drawWithCache {
    val step = 3.dp2px(density)
    val line = 1.dp2px(density)
    val vignette = Brush.radialGradient(
        colors = listOf(Color.Transparent, Color.Black.copy(alpha = 0.4f)),
        center = Offset(size.width / 2, size.height / 2),
        radius = maxOf(size.width, size.height) * 0.75f,
    )
    onDrawWithContent {
        drawContent()
        var y = 0f
        while (y < size.height) {
            drawRect(Color.Black.copy(alpha = 0.08f), topLeft = Offset(0f, y), size = androidx.compose.ui.geometry.Size(size.width, line))
            y += step
        }
        drawRect(vignette)
        if (cfg.flicker && Random.nextFloat() < 0.04f) {
            drawRect(Color.Black.copy(alpha = 0.03f))
        }
    }
}

private fun Int.dp2px(density: Float) = this * density

/**
 * Glitch transition (DESIGN.md 5.3): horizontal slices shift, red/cyan split, one bright scanline.
 * [progress] returns 0..1 while playing and 1 when idle.
 */
fun Modifier.glitch(progress: () -> Float, cfg: MatrixConfig): Modifier = if (!cfg.full) this else drawWithContent {
    val p = progress()
    if (p >= 1f || p <= 0f) {
        drawContent()
        return@drawWithContent
    }
    val rnd = Random((p * 7).toInt() * 31 + 7)
    val slices = 9
    val h = size.height / slices
    val amp = (1f - p) * 12f * density
    if (p < 0.55f) {
        // Channel split copies.
        val split = 2f * density
        for ((tint, dx) in listOf(Color(0xFFFF0040) to -split, Color(0xFF00E5FF) to split)) {
            drawIntoCanvas { c ->
                c.saveLayer(Rect(Offset.Zero, size), Paint().apply { colorFilter = ColorFilter.tint(tint, BlendMode.Modulate); alpha = 0.45f })
            }
            translate(left = dx) { this@drawWithContent.drawContent() }
            drawIntoCanvas { it.restore() }
        }
    }
    for (i in 0 until slices) {
        val off = if (rnd.nextFloat() < 0.6f) (rnd.nextFloat() * 2 - 1) * amp else 0f
        clipRect(top = i * h, bottom = (i + 1) * h) {
            translate(left = off) { this@drawWithContent.drawContent() }
        }
    }
    if (p in 0.4f..0.6f) {
        drawRect(cfg.colors.head.copy(alpha = 0.5f), topLeft = Offset(0f, size.height * rnd.nextFloat()), size = androidx.compose.ui.geometry.Size(size.width, 2f * density))
    }
}

/** A bright sweep from top to bottom (connected moment). [progress] 0..1, idle at 1. */
fun Modifier.sweep(progress: () -> Float, color: Color): Modifier = drawWithContent {
    drawContent()
    val p = progress()
    if (p <= 0f || p >= 1f) return@drawWithContent
    val y = size.height * p
    drawRect(
        Brush.verticalGradient(listOf(Color.Transparent, color.copy(alpha = 0.18f), color.copy(alpha = 0.8f)), startY = y - 120f * density, endY = y),
        topLeft = Offset(0f, y - 120f * density),
        size = androidx.compose.ui.geometry.Size(size.width, 120f * density),
    )
}

/** Plays a one-shot 0→1 animation each time [key] changes. Returns the Animatable for draw-phase reads. */
@Composable
fun rememberOneShot(key: Any?, durationMs: Int, playOnStart: Boolean = false): Animatable<Float, *> {
    val a = remember { Animatable(1f) }
    val first = remember { booleanArrayOf(true) }
    LaunchedEffect(key) {
        val isFirst = first[0]
        first[0] = false
        if (durationMs <= 0 || (isFirst && !playOnStart)) return@LaunchedEffect
        a.snapTo(0f)
        a.animateTo(1f, tween(durationMs, easing = LinearEasing))
    }
    return a
}

@Composable
fun Layered(modifier: Modifier = Modifier, content: @Composable () -> Unit) = Box(modifier) { content() }
