package dev.korokmote.app.ui

import androidx.compose.foundation.border
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.layout.Box
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import dev.korokmote.app.data.Settings
import dev.korokmote.app.design.CutShape
import dev.korokmote.app.design.Haptics
import dev.korokmote.app.design.LocalMatrix
import dev.korokmote.app.design.MType
import dev.korokmote.app.design.rememberHaptics
import dev.korokmote.app.net.ConnectionManager
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.abs
import kotlin.math.hypot

/** Converts gestures into pointer, scroll, click and shortcut messages (ANDROID.md, gesture engine). */
class TouchpadEngine(private val conn: ConnectionManager, private val haptics: Haptics) {
    var settings = Settings()
    var density = 1f
    private var mx = 0f
    private var my = 0f
    private var sx = 0f
    private var sy = 0f
    private var zoomAcc = 0f
    var zooming = false
        private set

    // Visual feedback (ring buffers read in the draw phase).
    val trailX = FloatArray(14)
    val trailY = FloatArray(14)
    val trailT = LongArray(14)
    var trailHead = 0
    data class Ring(val x: Float, val y: Float, val t: Long, val kind: Int)
    val rings = ArrayDeque<Ring>()
    var dragging = false

    fun trail(p: Offset) {
        trailHead = (trailHead + 1) % trailX.size
        trailX[trailHead] = p.x
        trailY[trailHead] = p.y
        trailT[trailHead] = System.currentTimeMillis()
    }

    fun move(d: Offset, dtMs: Long) {
        val dx = d.x / density
        val dy = d.y / density
        val speed = hypot(dx, dy) / dtMs.coerceAtLeast(1)
        val f = settings.pointerSpeed * (1f + settings.acceleration * speed)
        mx += dx * f
        my += dy * f
    }

    fun scroll(d: Offset) {
        val sign = if (settings.naturalScroll) -1f else 1f
        sy += d.y / density * 3f * settings.scrollSpeed * sign
        sx += -d.x / density * 3f * settings.scrollSpeed * sign
    }

    fun zoom(spanDelta: Float) {
        if (!zooming) {
            zooming = true
            flush()
            conn.conn?.send("key", Msg.key("CTRL", "down"))
        }
        zoomAcc += spanDelta / density
        while (abs(zoomAcc) >= 40f) {
            val step = if (zoomAcc > 0) 120 else -120
            conn.conn?.scroll(0, step)
            zoomAcc -= if (zoomAcc > 0) 40f else -40f
        }
    }

    fun endZoom() {
        if (zooming) {
            zooming = false
            zoomAcc = 0f
            conn.conn?.send("key", Msg.key("CTRL", "up"))
        }
    }

    fun flush() {
        val c = conn.conn ?: run { mx = 0f; my = 0f; sx = 0f; sy = 0f; return }
        val ix = mx.toInt()
        val iy = my.toInt()
        if (ix != 0 || iy != 0) {
            c.pointer(ix, iy)
            mx -= ix
            my -= iy
        }
        val wx = sx.toInt()
        val wy = sy.toInt()
        if (wx != 0 || wy != 0) {
            c.scroll(wx, wy)
            sx -= wx
            sy -= wy
        }
    }

    fun click(button: Int, at: Offset) {
        flush()
        conn.conn?.button(button, true)
        conn.conn?.button(button, false)
        rings.addLast(Ring(at.x, at.y, System.currentTimeMillis(), button))
        if (rings.size > 6) rings.removeFirst()
        if (button == 2) haptics.double() else haptics.tick()
    }

    fun buttonDown(at: Offset) {
        flush()
        conn.conn?.button(1, true)
        dragging = true
        rings.addLast(Ring(at.x, at.y, System.currentTimeMillis(), 1))
        haptics.medium()
    }

    fun buttonUp() {
        flush()
        conn.conn?.button(1, false)
        dragging = false
    }

    fun combo(vararg keys: String) {
        flush()
        conn.fire("combo", Msg.combo(keys.toList()))
        haptics.click()
    }
}

/** The touchpad surface with gesture recognition and glyph feedback (DESIGN.md 5.5). */
@Composable
fun TouchpadSurface(conn: ConnectionManager, settings: Settings, modifier: Modifier = Modifier, hint: Boolean = true) {
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val haptics = rememberHaptics()
    val density = LocalDensity.current.density
    val engine = remember { TouchpadEngine(conn, haptics) }
    engine.settings = settings
    engine.density = density
    var frame by remember { mutableIntStateOf(0) }
    var showHint by remember { mutableStateOf(hint) }
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) {
        while (true) {
            withFrameNanos {
                engine.flush()
                val now = System.currentTimeMillis()
                if (cfg.animated && (now - engine.trailT[engine.trailHead] < 400 || engine.rings.any { now - it.t < 300 } || engine.dragging)) frame++
            }
        }
    }
    val slop = 8f * density
    Box(
        modifier
            .clip(CutShape)
            .border(1.dp, if (engine.dragging) c.bright else c.dim, CutShape)
            .semantics { contentDescription = "Touchpad. One finger moves, tap clicks, two fingers scroll, two-finger tap right-clicks." }
            .pointerInput(Unit) {
                var lastTapUp = 0L
                var lastTapPos = Offset.Zero
                awaitEachGesture {
                    val first = awaitFirstDown(requireUnconsumed = false)
                    showHint = false
                    val t0 = first.uptimeMillis
                    val start = first.position
                    var maxPointers = 1
                    var moved = false
                    var total = Offset.Zero
                    var mode = 0 // 0 none, 1 move, 2 scroll, 3 pinch, 4 swipe3
                    var prevCentroid: Offset? = null
                    var prevSpan = 0f
                    var spanTotal = 0f
                    var twoTotal = Offset.Zero
                    var swipe = Offset.Zero
                    var lastTime = t0
                    var velY = 0f
                    var velX = 0f
                    val drag = t0 - lastTapUp < 280 && (start - lastTapPos).getDistance() < 48f * density
                    if (drag) engine.buttonDown(start)
                    engine.trail(start)
                    while (true) {
                        val ev = awaitPointerEvent()
                        val pressed = ev.changes.filter { it.pressed }
                        if (pressed.isEmpty()) {
                            ev.changes.forEach { it.consume() }
                            break
                        }
                        if (pressed.size > maxPointers) maxPointers = pressed.size
                        val now = pressed.first().uptimeMillis
                        val dt = (now - lastTime).coerceAtLeast(1)
                        lastTime = now
                        when (pressed.size) {
                            1 -> if (maxPointers == 1 || mode == 1) {
                                val ch = pressed[0]
                                val d = ch.position - ch.previousPosition
                                total += d
                                if (!moved && total.getDistance() > slop) moved = true
                                if (moved) {
                                    mode = 1
                                    engine.move(d, dt)
                                    engine.trail(ch.position)
                                }
                            }
                            2 -> {
                                val a = pressed[0].position
                                val b = pressed[1].position
                                val centroid = (a + b) / 2f
                                val span = (a - b).getDistance()
                                val pc = prevCentroid
                                if (pc != null) {
                                    val dC = centroid - pc
                                    val dS = span - prevSpan
                                    twoTotal += dC
                                    spanTotal += dS
                                    if (mode == 0 || mode == 1) {
                                        if (abs(spanTotal) > 36f * density && abs(spanTotal) > twoTotal.getDistance() * 1.5f) mode = 3
                                        else if (twoTotal.getDistance() > slop) mode = 2
                                    }
                                    when (mode) {
                                        2 -> {
                                            moved = true
                                            engine.scroll(dC)
                                            velX = dC.x / dt
                                            velY = dC.y / dt
                                        }
                                        3 -> {
                                            moved = true
                                            engine.zoom(dS)
                                        }
                                    }
                                }
                                prevCentroid = centroid
                                prevSpan = span
                            }
                            else -> {
                                val centroid = pressed.fold(Offset.Zero) { acc, p -> acc + p.position } / pressed.size.toFloat()
                                val pc = prevCentroid
                                if (pc != null && maxPointers >= 3) {
                                    swipe += centroid - pc
                                    if (swipe.getDistance() > slop * 2) {
                                        moved = true
                                        mode = 4
                                    }
                                }
                                prevCentroid = centroid
                            }
                        }
                        ev.changes.forEach { it.consume() }
                    }
                    engine.endZoom()
                    val upTime = System.currentTimeMillis()
                    val duration = lastTime - t0
                    when {
                        drag -> {
                            engine.buttonUp()
                            lastTapUp = 0L
                        }
                        !moved && duration < 300 && settings.tapToClick -> {
                            val b = when (maxPointers) {
                                1 -> 1
                                2 -> 2
                                else -> 3
                            }
                            engine.click(b, start)
                            if (b == 1) {
                                lastTapUp = first.uptimeMillis + duration
                                lastTapPos = start
                            }
                        }
                        mode == 4 -> {
                            val s = swipe
                            if (abs(s.x) > abs(s.y)) {
                                if (s.x > 0) engine.combo("ALT", "TAB") else engine.combo("ALT", "SHIFT", "TAB")
                            } else if (s.y < 0) {
                                engine.combo("WIN", "TAB")
                            } else {
                                engine.combo("WIN", "D")
                            }
                        }
                        mode == 2 && (abs(velY) > 0.3f || abs(velX) > 0.3f) -> {
                            // Scroll inertia.
                            val vx0 = velX
                            val vy0 = velY
                            scope.launch {
                                var vx = vx0
                                var vy = vy0
                                var k = 0
                                while (k < 30 && (abs(vx) > 0.05f || abs(vy) > 0.05f)) {
                                    delay(16)
                                    engine.scroll(Offset(vx * 16, vy * 16))
                                    vx *= 0.9f
                                    vy *= 0.9f
                                    k++
                                }
                            }
                        }
                    }
                    if (upTime < 0) Unit
                }
            }
            .drawBehind {
                @Suppress("UNUSED_EXPRESSION") frame
                if (!cfg.animated) return@drawBehind
                val now = System.currentTimeMillis()
                // Trail of fading glyph squares.
                val n = engine.trailX.size
                for (i in 0 until n) {
                    val idx = (engine.trailHead - i + n) % n
                    val age = now - engine.trailT[idx]
                    if (age > 300) continue
                    val a = (1f - age / 300f) * (1f - i / n.toFloat())
                    val s = (6 - i * 0.3f).coerceAtLeast(2f) * density
                    drawRect(if (i == 0) c.head.copy(alpha = a) else c.bright.copy(alpha = a * 0.8f), Offset(engine.trailX[idx] - s / 2, engine.trailY[idx] - s / 2), Size(s, s))
                }
                // Tap rings: circle (left), square (right), diamond (middle).
                for (r in engine.rings) {
                    val age = now - r.t
                    if (age > 300) continue
                    val p = age / 300f
                    val rad = (12f + 40f * p) * density
                    val col = c.bright.copy(alpha = 1f - p)
                    when (r.kind) {
                        1 -> drawCircle(col, rad, Offset(r.x, r.y), style = Stroke(2f * density))
                        2 -> drawRect(col, Offset(r.x - rad, r.y - rad), Size(rad * 2, rad * 2), style = Stroke(2f * density))
                        else -> rotate(45f, Offset(r.x, r.y)) {
                            drawRect(col, Offset(r.x - rad, r.y - rad), Size(rad * 2, rad * 2), style = Stroke(2f * density))
                        }
                    }
                }
                if (engine.dragging) {
                    val pulse = ((now / 4) % 250) / 250f
                    drawRect(c.bright.copy(alpha = 0.12f + 0.1f * pulse), style = Stroke(6f * density))
                }
            },
        contentAlignment = Alignment.Center,
    ) {
        if (showHint) {
            Text(
                "1 finger :: move\ntap :: click   2-tap :: right\n2 fingers :: scroll   pinch :: zoom\ndouble-tap & hold :: drag\n3 fingers ← → :: switch apps",
                style = MType.micro, color = c.dim,
            )
        }
    }
}
