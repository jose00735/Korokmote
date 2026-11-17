package dev.korokmote.app.design

import android.content.Context
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.os.Build
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext

/** Haptic vocabulary from docs/DESIGN.md section 5.8. */
class Haptics(private val vibrator: Vibrator?, private val level: Int) {
    private fun play(effect: VibrationEffect) {
        val v = vibrator ?: return
        if (level <= 0 || !v.hasVibrator()) return
        runCatching { v.vibrate(effect) }
    }

    private fun oneShot(ms: Long, amp: Int) {
        val a = if (level == 1) (amp * 0.5).toInt().coerceAtLeast(1) else amp
        play(VibrationEffect.createOneShot(ms, a.coerceIn(1, 255)))
    }

    fun tick() = if (level == 1) oneShot(8, 60) else play(VibrationEffect.createPredefined(VibrationEffect.EFFECT_TICK))
    fun click() = play(VibrationEffect.createPredefined(if (level == 1) VibrationEffect.EFFECT_TICK else VibrationEffect.EFFECT_CLICK))
    fun double() = play(VibrationEffect.createPredefined(VibrationEffect.EFFECT_DOUBLE_CLICK))
    fun medium() = oneShot(28, 160)
    fun heavy() = play(VibrationEffect.createPredefined(VibrationEffect.EFFECT_HEAVY_CLICK))
    fun longLow() = oneShot(180, 70)
    fun ramp(step: Float) = oneShot(6, (40 + step * 180).toInt())
}

fun vibratorOf(ctx: Context): Vibrator? = if (Build.VERSION.SDK_INT >= 31) {
    (ctx.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as? VibratorManager)?.defaultVibrator
} else {
    @Suppress("DEPRECATION")
    ctx.getSystemService(Context.VIBRATOR_SERVICE) as? Vibrator
}

@Composable
fun rememberHaptics(): Haptics {
    val ctx = LocalContext.current
    val level = LocalMatrix.current.haptics
    return remember(level) { Haptics(vibratorOf(ctx), level) }
}
