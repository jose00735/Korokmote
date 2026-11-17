package dev.korokmote.app

import android.app.Application
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import dev.korokmote.app.data.HostStore
import dev.korokmote.app.data.SettingsStore
import dev.korokmote.app.net.ConnState
import dev.korokmote.app.net.ConnectionManager
import dev.korokmote.app.net.Discovery
import dev.korokmote.app.protocol.Phrase
import dev.korokmote.app.service.ConnectionService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** Manual dependency graph. */
class KorokApp : Application() {
    lateinit var hosts: HostStore
    lateinit var settings: SettingsStore
    lateinit var discovery: Discovery
    lateinit var connection: ConnectionManager
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var backgroundTimer: Job? = null

    override fun onCreate() {
        super.onCreate()
        instance = this
        Phrase.words = assets.open("wordlist.txt").bufferedReader().readLines().map { it.trim() }.filter { it.isNotEmpty() }
        hosts = HostStore(this)
        settings = SettingsStore(this)
        discovery = Discovery(this, scope)
        connection = ConnectionManager(this, scope, hosts, discovery)

        scope.launch {
            connection.state.collect { st ->
                when (st) {
                    is ConnState.Connecting, is ConnState.Online, is ConnState.Reconnecting -> ConnectionService.start(this@KorokApp)
                    else -> ConnectionService.stop(this@KorokApp)
                }
            }
        }
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStop(owner: LifecycleOwner) = startBackgroundTimer()
            override fun onStart(owner: LifecycleOwner) {
                backgroundTimer?.cancel()
            }
        })
    }

    /** Disconnects after the configured time in the background (notification actions reset it). */
    fun startBackgroundTimer() {
        backgroundTimer?.cancel()
        val minutes = settings.state.value.backgroundMinutes
        if (minutes <= 0) return
        backgroundTimer = scope.launch {
            delay(minutes * 60_000L)
            connection.disconnect()
        }
    }

    fun noteNotificationUse() {
        if (!ProcessLifecycleOwner.get().lifecycle.currentState.isAtLeast(androidx.lifecycle.Lifecycle.State.STARTED)) {
            startBackgroundTimer()
        }
    }

    companion object {
        lateinit var instance: KorokApp
            private set
    }
}
