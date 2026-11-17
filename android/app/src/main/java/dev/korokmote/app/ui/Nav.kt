package dev.korokmote.app.ui

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import dev.korokmote.app.net.PairPayload

enum class Tab { PAD, KEYS, MEDIA, REMOTES, MORE }

sealed interface Route {
    data object Boot : Route
    data object Hosts : Route
    data class Pair(val host: String? = null, val port: Int? = null, val manual: Boolean = false) : Route
    data class Main(val tab: Tab = Tab.PAD) : Route
    data class Remote(val id: String) : Route
    data object Power : Route
    data class Files(val tab: Int = 0) : Route
    data object Clipboard : Route
    data object Settings : Route
}

/** Minimal back stack; route changes trigger the glitch transition. */
class Nav {
    val stack = mutableStateListOf<Route>(Route.Boot)
    var pendingPair by mutableStateOf<PairPayload?>(null)

    val current: Route get() = stack.last()

    fun push(r: Route) {
        if (stack.last() != r) stack.add(r)
    }

    fun pop(): Boolean {
        if (stack.size <= 1) return false
        stack.removeAt(stack.lastIndex)
        return true
    }

    fun replaceTop(r: Route) {
        stack[stack.lastIndex] = r
    }

    fun replaceAll(r: Route) {
        stack.clear()
        stack.add(r)
    }
}
