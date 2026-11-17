package dev.korokmote.app.design

import android.graphics.Paint
import android.graphics.Typeface
import androidx.compose.foundation.Canvas
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.sp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.repeatOnLifecycle
import dev.korokmote.app.data.Intensity
import dev.korokmote.app.data.RainDensity
import kotlinx.coroutines.flow.MutableStateFlow
import kotlin.random.Random

/** Global mood for the rain, driven by connection events (DESIGN.md 5.1). */
enum class RainMood { NORMAL, FAST, SLOW, FROZEN, DRAIN }

object RainBus {
    val mood = MutableStateFlow(RainMood.NORMAL)
}

private class Stream(var y: Float, var speed: Float, var length: Int, var wait: Float, var active: Boolean, var glyphs: CharArray)

private class RainState {
    var streams: Array<Stream> = emptyArray()
    var cols = 0
    var rows = 0
    var cell = 0f
    var glyphSet = ""
    var density = RainDensity.NORMAL
    val rnd = Random(42)

    fun ensure(size: Size, cellPx: Float, glyphs: String, d: RainDensity) {
        val c = (size.width / cellPx).toInt() + 1
        val r = (size.height / cellPx).toInt() + 1
        if (c == cols && r == rows && glyphs == glyphSet && d == density && streams.isNotEmpty()) return
        cols = c; rows = r; cell = cellPx; glyphSet = glyphs; density = d
        val every = when (d) {
            RainDensity.SPARSE -> 3
            RainDensity.NORMAL -> 2
            RainDensity.DENSE -> 1
        }
        streams = Array(c) { i ->
            Stream(
                y = rnd.nextFloat() * r * 1.4f - r * 0.4f,
                speed = 4f + rnd.nextFloat() * 10f,
                length = 8 + rnd.nextInt(21),
                wait = rnd.nextFloat() * 3f,
                active = i % every == 0,
                glyphs = CharArray(r + 30) { glyphs[rnd.nextInt(glyphs.length)] },
            )
        }
    }

    fun step(dt: Float, mood: RainMood) {
        val mult = when (mood) {
            RainMood.NORMAL -> 1f
            RainMood.FAST -> 3f
            RainMood.SLOW -> 0.35f
            RainMood.FROZEN -> 0f
            RainMood.DRAIN -> 2.2f
        }
        if (mult == 0f) return
        for (s in streams) {
            if (!s.active) continue
            if (s.wait > 0f) {
                s.wait -= dt
                continue
            }
            s.y += s.speed * dt * mult
            if (s.y - s.length > rows) {
                if (mood == RainMood.DRAIN) {
                    s.wait = 9999f // stays empty until the mood changes
                } else {
                    s.y = 0f
                    s.speed = 4f + rnd.nextFloat() * 10f
                    s.length = 8 + rnd.nextInt(21)
                    s.wait = rnd.nextFloat() * 2f
                }
            }
            // Mutate ~3% of glyphs per tick.
            val n = (s.glyphs.size * 0.03f).toInt().coerceAtLeast(1)
            repeat(n) { s.glyphs[rnd.nextInt(s.glyphs.size)] = glyphSet[rnd.nextInt(glyphSet.length)] }
        }
        if (mood != RainMood.DRAIN) {
            for (s in streams) if (s.wait > 9000f) {
                s.wait = rnd.nextFloat() * 1.5f
                s.y = 0f
            }
        }
    }
}

/**
 * Digital rain background. Ticks at 30 fps on its own, only while the screen is resumed,
 * and only invalidates its own draw (no recomposition of content).
 */
@Composable
fun DigitalRain(modifier: Modifier = Modifier, alpha: Float = 1f) {
    val cfg = LocalMatrix.current
    if (cfg.intensity == Intensity.OFF || alpha <= 0f) return
    val cellPx = with(LocalDensity.current) { 14.sp.toPx() }
    val state = remember { RainState() }
    var tick by remember { mutableIntStateOf(0) }
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val paint = remember {
        Paint(Paint.ANTI_ALIAS_FLAG).apply {
            typeface = Typeface.MONOSPACE
            textAlign = Paint.Align.CENTER
        }
    }
    if (cfg.full) {
        LaunchedEffect(lifecycle) {
            lifecycle.repeatOnLifecycle(Lifecycle.State.RESUMED) {
                var last = 0L
                while (true) {
                    withFrameNanos { now ->
                        if (last == 0L) last = now
                        val dt = (now - last) / 1e9f
                        if (dt >= 1f / 30f) {
                            state.step(dt.coerceAtMost(0.1f), RainBus.mood.value)
                            last = now
                            tick++
                        }
                    }
                }
            }
        }
    }
    Canvas(modifier) {
        @Suppress("UNUSED_EXPRESSION") tick
        state.ensure(size, cellPx, cfg.glyphs, cfg.density)
        if (!cfg.full && tick == 0) state.step(0f, RainMood.NORMAL)
        drawRain(state, paint, cfg, alpha)
    }
}

private fun DrawScope.drawRain(state: RainState, paint: Paint, cfg: MatrixConfig, alpha: Float) {
    paint.textSize = state.cell * 0.92f
    val head = cfg.colors.head.toArgb()
    val bright = cfg.colors.bright.toArgb()
    val ph = cfg.colors.phosphor.toArgb()
    drawIntoCanvas { canvas ->
        val nc = canvas.nativeCanvas
        for ((i, s) in state.streams.withIndex()) {
            if (!s.active || s.wait > 0f) continue
            val x = i * state.cell + state.cell / 2
            val headRow = s.y.toInt()
            for (j in 0 until s.length) {
                val row = headRow - j
                if (row < 0 || row > state.rows) continue
                val color = when {
                    j == 0 -> head
                    j < 3 -> bright
                    else -> ph
                }
                val fade = if (j < 3) 1f else (1f - (j - 3f) / (s.length - 2f)).coerceIn(0.05f, 1f)
                paint.color = color
                paint.alpha = (255 * fade * alpha).toInt().coerceIn(0, 255)
                nc.drawText(s.glyphs, row % s.glyphs.size, 1, x, (row + 1) * state.cell - state.cell * 0.18f, paint)
            }
        }
    }
}
