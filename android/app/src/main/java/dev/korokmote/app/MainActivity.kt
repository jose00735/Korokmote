package dev.korokmote.app

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.KeyEvent
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.core.content.ContextCompat
import dev.korokmote.app.net.PairPayload
import dev.korokmote.app.ui.AppRoot
import dev.korokmote.app.ui.Nav
import dev.korokmote.app.ui.Route
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

class MainActivity : ComponentActivity() {
    private val app get() = application as KorokApp
    private val nav = Nav()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        handleIntent(intent)
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
        setContent { AppRoot(app, nav) }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        when {
            intent?.action == ACTION_CONNECT -> {
                val id = intent.getStringExtra(EXTRA_HOST) ?: return
                app.hosts.get(id)?.let { app.connection.connect(it) }
                nav.replaceAll(Route.Hosts)
            }
            intent?.action == Intent.ACTION_VIEW && intent.data?.scheme == "korokmote" -> {
                PairPayload.parse(intent.data.toString())?.let { nav.pendingPair = it }
            }
        }
    }

    /** Hardware volume keys control the PC while connected (setting). */
    override fun dispatchKeyEvent(event: KeyEvent): Boolean {
        val c = app.connection
        if (app.settings.state.value.volumeKeys && c.online &&
            (event.keyCode == KeyEvent.KEYCODE_VOLUME_UP || event.keyCode == KeyEvent.KEYCODE_VOLUME_DOWN)
        ) {
            if (event.action == KeyEvent.ACTION_DOWN) {
                val delta = if (event.keyCode == KeyEvent.KEYCODE_VOLUME_UP) 0.02 else -0.02
                c.fire("audio.set", buildJsonObject { put("device", "default"); put("delta", delta) })
            }
            return true
        }
        return super.dispatchKeyEvent(event)
    }

    companion object {
        const val ACTION_CONNECT = "dev.korokmote.CONNECT"
        const val EXTRA_HOST = "host"
    }
}
