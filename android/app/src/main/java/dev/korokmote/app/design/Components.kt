package dev.korokmote.app.design

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlin.math.PI
import kotlin.math.atan2
import kotlin.math.roundToInt

enum class ButtonStyle { DEFAULT, PRIMARY, DANGER }

/** Section header in the `[ MEDIA ]` style. */
@Composable
fun SectionHeader(text: String, modifier: Modifier = Modifier) {
    DecodeText("[ $text ]", modifier.padding(top = 18.dp, bottom = 8.dp), style = MType.micro, color = LocalMatrix.current.colors.phosphor)
}

/**
 * Cut-corner terminal button (DESIGN.md 5.6): border snaps bright on touch-down,
 * fill inverts briefly on click, label jitters one glyph. Optional long-press and auto-repeat.
 */
@Composable
fun TerminalButton(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    style: ButtonStyle = ButtonStyle.DEFAULT,
    onLongClick: (() -> Unit)? = null,
    repeat: Boolean = false,
    enabled: Boolean = true,
    minHeight: Dp = 56.dp,
    textStyle: TextStyle = MType.button,
    latched: Boolean = false,
    description: String? = null,
) {
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val haptics = rememberHaptics()
    val scope = rememberCoroutineScope()
    var pressed by remember { mutableStateOf(false) }
    val flash = remember { Animatable(0f) }
    var jitter by remember { mutableIntStateOf(-1) }
    var repeated by remember { mutableStateOf(false) }
    val click by rememberUpdatedState(onClick)
    val long by rememberUpdatedState(onLongClick)
    val accent = when (style) {
        ButtonStyle.DANGER -> c.danger
        else -> c.bright
    }
    val rest = when (style) {
        ButtonStyle.DANGER -> c.danger.copy(alpha = 0.55f)
        ButtonStyle.PRIMARY -> c.bright.copy(alpha = 0.8f)
        ButtonStyle.DEFAULT -> c.dim
    }
    fun fire() {
        click()
        haptics.tick()
        if (cfg.animated) {
            scope.launch {
                flash.snapTo(1f)
                flash.animateTo(0f, tween(cfg.dur(80)))
            }
            if (cfg.full && label.isNotEmpty()) {
                scope.launch {
                    jitter = (label.indices).random()
                    delay(60)
                    jitter = -1
                }
            }
        }
    }
    val shown = if (jitter in label.indices) label.replaceRange(jitter, jitter + 1, cfg.glyphs.random().toString()) else label
    val inverted = latched
    Box(
        modifier
            .heightIn(min = minHeight)
            .semantics {
                role = Role.Button
                contentDescription = description ?: label
            }
            .clip(CutShape)
            .drawBehind {
                val base = if (inverted) accent else if (style == ButtonStyle.PRIMARY) c.raised else c.void
                drawRect(lerp(base, if (inverted) c.void else accent, flash.value))
            }
            .border(1.dp, if (pressed || inverted) accent else rest, CutShape)
            .then(
                if (!enabled) {
                    Modifier
                } else {
                    Modifier.pointerInput(repeat, onLongClick != null) {
                        detectTapGestures(
                            onPress = {
                                pressed = true
                                repeated = false
                                var job: Job? = null
                                if (repeat) {
                                    job = scope.launch {
                                        delay(380)
                                        while (true) {
                                            repeated = true
                                            fire()
                                            delay(110)
                                        }
                                    }
                                }
                                tryAwaitRelease()
                                job?.cancel()
                                pressed = false
                            },
                            onTap = { if (!repeated) fire() },
                            onLongPress = if (long != null) {
                                { haptics.medium(); long?.invoke() }
                            } else {
                                null
                            },
                        )
                    }
                },
            )
            .padding(horizontal = 10.dp, vertical = 8.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(
            shown,
            style = textStyle,
            color = when {
                !enabled -> c.dim
                inverted -> c.void
                else -> lerp(accent, c.void, flash.value)
            },
            textAlign = TextAlign.Center,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** A row of buttons sharing the width. */
@Composable
fun ButtonRow(modifier: Modifier = Modifier, content: @Composable RowScope.() -> Unit) {
    Row(modifier.fillMaxWidth().padding(vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp), content = content)
}

/** Keyboard key cap, with a latch state for modifiers. */
@Composable
fun KeyCap(label: String, onClick: () -> Unit, modifier: Modifier = Modifier, latched: Boolean = false, locked: Boolean = false, repeat: Boolean = false) {
    val cfg = LocalMatrix.current
    val blink = if (locked && cfg.animated) {
        rememberInfiniteTransition(label = "lock").animateFloat(0f, 1f, infiniteRepeatable(tween(530), RepeatMode.Reverse), label = "b").value
    } else {
        1f
    }
    Box(modifier) {
        TerminalButton(label, onClick, Modifier.fillMaxWidth(), latched = latched || locked, repeat = repeat, minHeight = 48.dp, textStyle = MType.mono)
        if (locked) {
            Box(Modifier.align(Alignment.TopEnd).padding(4.dp).size(6.dp).background(cfg.colors.void.copy(alpha = blink)))
        }
    }
}

/**
 * Hold-to-confirm button (DESIGN.md 5.6): a ring of glyph segments fills over 900 ms,
 * haptic ticks get closer, release early drains it, completion flashes.
 */
@Composable
fun HoldRing(label: String, onComplete: () -> Unit, modifier: Modifier = Modifier, danger: Boolean = false, size: Dp = 112.dp) {
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val color = if (danger) c.danger else c.bright
    val haptics = rememberHaptics()
    val progress = remember { Animatable(0f) }
    val flash = remember { Animatable(0f) }
    val scope = rememberCoroutineScope()
    val done by rememberUpdatedState(onComplete)
    Column(modifier, horizontalAlignment = Alignment.CenterHorizontally) {
        Box(
            Modifier
                .size(size)
                .semantics {
                    role = Role.Button
                    contentDescription = "$label. Press and hold to confirm."
                }
                .pointerInput(Unit) {
                    detectTapGestures(onPress = {
                        val fill = scope.launch {
                            var lastTick = 0f
                            progress.animateTo(1f, tween(900, easing = LinearEasing)) {
                                val gap = 0.18f - value * 0.14f
                                if (value - lastTick > gap) {
                                    lastTick = value
                                    haptics.ramp(value)
                                }
                            }
                            haptics.heavy()
                            done()
                            flash.snapTo(1f)
                            flash.animateTo(0f, tween(cfg.dur(300)))
                            progress.snapTo(0f)
                        }
                        tryAwaitRelease()
                        if (progress.value < 1f) {
                            fill.cancel()
                            scope.launch { progress.animateTo(0f, tween(cfg.dur(250).coerceAtLeast(1))) }
                        }
                    })
                }
                .drawBehind {
                    val segs = 36
                    val stroke = 5.dp.toPx()
                    val r = this.size.minDimension / 2 - stroke
                    val filled = (progress.value * segs).roundToInt()
                    for (i in 0 until segs) {
                        val start = -90f + i * (360f / segs)
                        drawArc(
                            color = if (i < filled) color else c.dim,
                            startAngle = start,
                            sweepAngle = 360f / segs * 0.62f,
                            useCenter = false,
                            topLeft = Offset(center.x - r, center.y - r),
                            size = Size(r * 2, r * 2),
                            style = Stroke(stroke, cap = StrokeCap.Butt),
                        )
                    }
                    if (flash.value > 0f) drawCircle(color.copy(alpha = 0.6f * flash.value), radius = r)
                },
            contentAlignment = Alignment.Center,
        ) {
            Text(label, style = MType.micro, color = color, textAlign = TextAlign.Center, modifier = Modifier.widthIn(max = size - 28.dp))
        }
    }
}

/** Slider drawn as block glyphs ▮▮▮▯▯ (DESIGN.md 4). Drag or tap to set. */
@Composable
fun GlyphSlider(value: Float, onChange: (Float) -> Unit, modifier: Modifier = Modifier, steps: Int = 20, label: String? = null) {
    val c = LocalMatrix.current.colors
    val change by rememberUpdatedState(onChange)
    Column(modifier) {
        if (label != null) {
            Text("$label :: ${(value * 100).roundToInt()}%", style = MType.micro, color = c.phosphor)
            Spacer(Modifier.height(4.dp))
        }
        Box(
            Modifier
                .fillMaxWidth()
                .height(36.dp)
                .semantics { contentDescription = "${label ?: "Slider"} ${(value * 100).roundToInt()} percent" }
                .pointerInput(Unit) {
                    detectTapGestures { change((it.x / size.width).coerceIn(0f, 1f)) }
                }
                .pointerInput(Unit) {
                    detectDragGestures { ch, _ -> change((ch.position.x / size.width).coerceIn(0f, 1f)) }
                }
                .drawBehind {
                    val w = size.width / steps
                    val on = (value * steps).roundToInt()
                    for (i in 0 until steps) {
                        drawRect(
                            if (i < on) c.bright else c.dim,
                            topLeft = Offset(i * w + w * 0.15f, size.height * 0.2f),
                            size = Size(w * 0.7f, size.height * 0.6f),
                        )
                    }
                },
        )
    }
}

/** Circular segmented volume control. Drag around the ring to set the level. */
@Composable
fun VolumeRing(level: Float, muted: Boolean, onSet: (Float) -> Unit, modifier: Modifier = Modifier, size: Dp = 220.dp) {
    val c = LocalMatrix.current.colors
    val set by rememberUpdatedState(onSet)
    Box(
        modifier
            .size(size)
            .semantics { contentDescription = "Volume ${(level * 100).roundToInt()} percent" + if (muted) ", muted" else "" }
            .pointerInput(Unit) {
                fun at(p: Offset) {
                    val cx = this.size.width / 2f
                    val cy = this.size.height / 2f
                    var deg = Math.toDegrees(atan2((p.y - cy).toDouble(), (p.x - cx).toDouble())).toFloat() - 135f
                    while (deg < 0) deg += 360f
                    if (deg <= 270f) set(deg / 270f) else if (deg < 315f) set(1f) else set(0f)
                }
                detectDragGestures(onDragStart = { at(it) }) { ch, _ -> at(ch.position) }
            }
            .drawBehind {
                val segs = 40
                val stroke = 14.dp.toPx()
                val r = this.size.minDimension / 2 - stroke
                val filled = (level * segs).roundToInt()
                for (i in 0 until segs) {
                    val start = 135f + i * (270f / segs)
                    drawArc(
                        if (muted) (if (i < filled) c.phosphor else c.dim) else if (i < filled) c.bright else c.dim,
                        start, 270f / segs * 0.7f, false,
                        topLeft = Offset(center.x - r, center.y - r), size = Size(r * 2, r * 2),
                        style = Stroke(stroke),
                    )
                }
            },
        contentAlignment = Alignment.Center,
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            DecodeText(if (muted) "MUTE" else "${(level * 100).roundToInt()}", style = MType.display, color = if (muted) c.warn else c.bright)
            Text("VOLUME", style = MType.micro, color = c.phosphor)
        }
    }
}

/** List row with an index prefix (`00 01 02`). */
@Composable
fun TerminalRow(
    index: Int,
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    trailing: String? = null,
    titleColor: Color = LocalMatrix.current.colors.bright,
    onLongClick: (() -> Unit)? = null,
    onClick: () -> Unit,
) {
    val c = LocalMatrix.current.colors
    val haptics = rememberHaptics()
    Row(
        modifier
            .fillMaxWidth()
            .combinedClickable(onLongClick = onLongClick?.let { { haptics.medium(); it() } }) { haptics.tick(); onClick() }
            .drawBehind { drawLine(c.dim, Offset(0f, size.height), Offset(size.width, size.height), 1f) }
            .padding(vertical = 12.dp, horizontal = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(index.toString().padStart(2, '0'), style = MType.mono, color = c.phosphor, modifier = Modifier.width(30.dp))
        Column(Modifier.weight(1f)) {
            Text(title, style = MType.body, color = titleColor, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (subtitle != null) Text(subtitle, style = MType.micro, color = c.phosphor, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        if (trailing != null) Text(trailing, style = MType.micro, color = c.phosphor)
    }
}

/** Text field styled as a terminal prompt. */
@Composable
fun PromptField(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "",
    keyboardType: KeyboardType = KeyboardType.Text,
    imeAction: ImeAction = ImeAction.Done,
    onDone: (() -> Unit)? = null,
    singleLine: Boolean = true,
) {
    val c = LocalMatrix.current.colors
    Row(
        modifier
            .fillMaxWidth()
            .border(1.dp, c.dim, CutShape)
            .padding(horizontal = 12.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("> ", style = MType.body, color = c.phosphor)
        Box(Modifier.weight(1f)) {
            if (value.isEmpty()) Text(placeholder, style = MType.body, color = c.dim)
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = singleLine,
                textStyle = MType.body.copy(color = c.bright),
                cursorBrush = SolidColor(c.bright),
                keyboardOptions = KeyboardOptions(keyboardType = keyboardType, imeAction = imeAction, autoCorrectEnabled = false),
                keyboardActions = KeyboardActions(onAny = { onDone?.invoke() }),
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

/** Dialog with a `[ TITLE ]` bar and a glitch-in entrance. */
@Composable
fun HexDialog(title: String, onDismiss: () -> Unit, content: @Composable () -> Unit) {
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val anim = rememberOneShot(title, cfg.dur(160), playOnStart = true)
    Dialog(onDismissRequest = onDismiss) {
        Column(
            Modifier
                .fillMaxWidth()
                .glitch({ anim.value }, cfg)
                .clip(CutShape)
                .background(c.surface)
                .border(1.dp, c.bright, CutShape)
                .padding(18.dp),
        ) {
            DecodeText("[ $title ]", style = MType.title)
            Spacer(Modifier.height(12.dp))
            content()
        }
    }
}

enum class Signal { ONLINE, ASLEEP, UNKNOWN }

/** Status dot; pulses when online. */
@Composable
fun SignalDot(signal: Signal, modifier: Modifier = Modifier) {
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val pulse = if (signal == Signal.ONLINE && cfg.animated) {
        rememberInfiniteTransition(label = "dot").animateFloat(0.35f, 1f, infiniteRepeatable(tween(900), RepeatMode.Reverse), label = "p").value
    } else {
        1f
    }
    val color = when (signal) {
        Signal.ONLINE -> c.bright
        Signal.ASLEEP -> c.warn
        Signal.UNKNOWN -> c.dim
    }
    Box(
        modifier
            .size(10.dp)
            .semantics { contentDescription = signal.name.lowercase() }
            .drawBehind {
                drawRect(color.copy(alpha = pulse))
                if (signal == Signal.ONLINE) drawRect(color.copy(alpha = 0.25f * pulse), topLeft = Offset(-3.dp.toPx(), -3.dp.toPx()), size = Size(size.width + 6.dp.toPx(), size.height + 6.dp.toPx()))
            },
    )
}

/** Small clickable label used for inline actions like `[ + PAIR ]`. */
@Composable
fun BracketAction(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, color: Color = LocalMatrix.current.colors.bright) {
    val haptics = rememberHaptics()
    Text(
        "[ $text ]",
        style = MType.button,
        color = color,
        modifier = modifier
            .clickable { haptics.tick(); onClick() }
            .padding(horizontal = 6.dp, vertical = 10.dp),
    )
}

@Suppress("unused")
private const val TAU = (2 * PI).toFloat()
