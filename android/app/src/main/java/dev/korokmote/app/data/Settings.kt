package dev.korokmote.app.data

import android.content.Context
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

enum class Intensity { FULL, REDUCED, OFF }
enum class RainDensity { SPARSE, NORMAL, DENSE }
enum class GlyphSet { KATAKANA, BINARY, HEX }
enum class Accent { GREEN, AMBER, CYAN, RED }

data class Settings(
    val pointerSpeed: Float = 1.4f,
    val acceleration: Float = 0.8f,
    val scrollSpeed: Float = 1.0f,
    val naturalScroll: Boolean = false,
    val tapToClick: Boolean = true,
    val haptics: Int = 2, // 0 off, 1 light, 2 normal
    val intensity: Intensity = Intensity.FULL,
    val density: RainDensity = RainDensity.NORMAL,
    val glyphs: GlyphSet = GlyphSet.KATAKANA,
    val accent: Accent = Accent.GREEN,
    val keepScreenOn: Boolean = true,
    val volumeKeys: Boolean = true,
    val crtFlicker: Boolean = false,
    val backgroundMinutes: Int = 10,
)

class SettingsStore(context: Context) {
    private val p = context.getSharedPreferences("settings", Context.MODE_PRIVATE)
    private val _state = MutableStateFlow(read())
    val state: StateFlow<Settings> = _state

    private inline fun <reified E : Enum<E>> enumOf(key: String, def: E): E =
        runCatching { enumValueOf<E>(p.getString(key, def.name)!!) }.getOrDefault(def)

    private fun read(): Settings {
        val d = Settings()
        return Settings(
            pointerSpeed = p.getFloat("pointerSpeed", d.pointerSpeed),
            acceleration = p.getFloat("acceleration", d.acceleration),
            scrollSpeed = p.getFloat("scrollSpeed", d.scrollSpeed),
            naturalScroll = p.getBoolean("naturalScroll", d.naturalScroll),
            tapToClick = p.getBoolean("tapToClick", d.tapToClick),
            haptics = p.getInt("haptics", d.haptics),
            intensity = enumOf("intensity", d.intensity),
            density = enumOf("density", d.density),
            glyphs = enumOf("glyphs", d.glyphs),
            accent = enumOf("accent", d.accent),
            keepScreenOn = p.getBoolean("keepScreenOn", d.keepScreenOn),
            volumeKeys = p.getBoolean("volumeKeys", d.volumeKeys),
            crtFlicker = p.getBoolean("crtFlicker", d.crtFlicker),
            backgroundMinutes = p.getInt("backgroundMinutes", d.backgroundMinutes),
        )
    }

    fun update(f: (Settings) -> Settings) {
        val s = f(_state.value)
        p.edit()
            .putFloat("pointerSpeed", s.pointerSpeed)
            .putFloat("acceleration", s.acceleration)
            .putFloat("scrollSpeed", s.scrollSpeed)
            .putBoolean("naturalScroll", s.naturalScroll)
            .putBoolean("tapToClick", s.tapToClick)
            .putInt("haptics", s.haptics)
            .putString("intensity", s.intensity.name)
            .putString("density", s.density.name)
            .putString("glyphs", s.glyphs.name)
            .putString("accent", s.accent.name)
            .putBoolean("keepScreenOn", s.keepScreenOn)
            .putBoolean("volumeKeys", s.volumeKeys)
            .putBoolean("crtFlicker", s.crtFlicker)
            .putInt("backgroundMinutes", s.backgroundMinutes)
            .apply()
        _state.value = s
    }
}
