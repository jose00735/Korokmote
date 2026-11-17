package dev.korokmote.app.protocol

/** Key names from PROTOCOL.md section 8, grouped for the keyboard screen. */
object Keys {
    val navigation = listOf("ESC", "TAB", "ENTER", "BACKSPACE", "DELETE", "INSERT", "HOME", "END", "PGUP", "PGDN")
    val arrows = listOf("UP", "LEFT", "DOWN", "RIGHT")
    val modifiers = listOf("CTRL", "SHIFT", "ALT", "WIN")
    val function = (1..12).map { "F$it" }
    val system = listOf("PRTSC", "MENU", "CAPS", "NUMLOCK", "PAUSE")

    data class Shortcut(val label: String, val keys: List<String>)

    val shortcuts = listOf(
        Shortcut("COPY", listOf("CTRL", "C")),
        Shortcut("PASTE", listOf("CTRL", "V")),
        Shortcut("CUT", listOf("CTRL", "X")),
        Shortcut("UNDO", listOf("CTRL", "Z")),
        Shortcut("REDO", listOf("CTRL", "Y")),
        Shortcut("ALL", listOf("CTRL", "A")),
        Shortcut("SAVE", listOf("CTRL", "S")),
        Shortcut("FIND", listOf("CTRL", "F")),
        Shortcut("NEW TAB", listOf("CTRL", "T")),
        Shortcut("CLOSE", listOf("ALT", "F4")),
        Shortcut("DESKTOP", listOf("WIN", "D")),
        Shortcut("LOCK", listOf("WIN", "L")),
        Shortcut("RUN", listOf("WIN", "R")),
        Shortcut("SNIP", listOf("WIN", "SHIFT", "S")),
        Shortcut("TASK MGR", listOf("CTRL", "SHIFT", "ESC")),
        Shortcut("EMOJI", listOf("WIN", "PERIOD")),
    )

    val display = mapOf(
        "BACKSPACE" to "BKSP", "DELETE" to "DEL", "INSERT" to "INS", "PGUP" to "PG↑", "PGDN" to "PG↓",
        "UP" to "↑", "DOWN" to "↓", "LEFT" to "←", "RIGHT" to "→", "ENTER" to "ENT",
    )

    fun label(k: String) = display[k] ?: k
}
