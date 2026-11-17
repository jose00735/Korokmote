package dev.korokmote.app.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.graphics.drawable.Icon
import android.os.Build
import android.os.IBinder
import dev.korokmote.app.KorokApp
import dev.korokmote.app.MainActivity
import dev.korokmote.app.R
import dev.korokmote.app.net.ConnState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.launch
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** Keeps the session alive while the app is in the background and offers media controls. */
class ConnectionService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private val app get() = application as KorokApp

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        val nm = getSystemService(NotificationManager::class.java)
        nm.createNotificationChannel(
            NotificationChannel(CHANNEL, getString(R.string.channel_name), NotificationManager.IMPORTANCE_LOW).apply {
                setShowBadge(false)
            },
        )
        val first = build(app.connection.state.value, -1)
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(ID, first, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE)
        } else {
            startForeground(ID, first)
        }
        scope.launch {
            combine(app.connection.state, app.connection.rtt) { s, r -> s to r }.collect { (s, r) ->
                nm.notify(ID, build(s, r))
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val c = app.connection
        when (intent?.action) {
            A_PREV -> c.fire("media", buildJsonObject { put("a", "prev") })
            A_PLAY -> c.fire("media", buildJsonObject { put("a", "playpause") })
            A_NEXT -> c.fire("media", buildJsonObject { put("a", "next") })
            A_VDOWN -> c.fire("audio.set", buildJsonObject { put("device", "default"); put("delta", -0.04) })
            A_VUP -> c.fire("audio.set", buildJsonObject { put("device", "default"); put("delta", 0.04) })
            A_STOP -> c.disconnect()
        }
        if (intent?.action != null) app.noteNotificationUse()
        return START_NOT_STICKY
    }

    private fun action(icon: Int, title: String, act: String): Notification.Action {
        val pi = PendingIntent.getService(this, act.hashCode(), Intent(this, ConnectionService::class.java).setAction(act), PendingIntent.FLAG_IMMUTABLE)
        return Notification.Action.Builder(Icon.createWithResource(this, icon), title, pi).build()
    }

    private fun build(s: ConnState, rtt: Long): Notification {
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val name = s.hostOrNull?.name ?: "Korokmote"
        val text = when (s) {
            is ConnState.Online -> "> online" + if (rtt >= 0) " :: $rtt ms" else ""
            is ConnState.Connecting -> "> connecting…"
            is ConnState.Reconnecting -> "> reconnecting (attempt ${s.attempt})"
            is ConnState.Failed -> "> ${s.reason}"
            ConnState.Idle -> "> idle"
        }
        return Notification.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_k)
            .setContentTitle(name)
            .setContentText(text)
            .setContentIntent(open)
            .setOngoing(true)
            .setColor(0xFF00FF41.toInt())
            .setCategory(Notification.CATEGORY_SERVICE)
            .addAction(action(R.drawable.ic_n_prev, "Previous", A_PREV))
            .addAction(action(R.drawable.ic_n_play, "Play/pause", A_PLAY))
            .addAction(action(R.drawable.ic_n_next, "Next", A_NEXT))
            .addAction(action(R.drawable.ic_n_vdown, "Volume down", A_VDOWN))
            .addAction(action(R.drawable.ic_n_vup, "Volume up", A_VUP))
            .setStyle(Notification.MediaStyle().setShowActionsInCompactView(0, 1, 2))
            .build()
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    companion object {
        private const val CHANNEL = "connection"
        private const val ID = 1
        private const val A_PREV = "dev.korokmote.PREV"
        private const val A_PLAY = "dev.korokmote.PLAY"
        private const val A_NEXT = "dev.korokmote.NEXT"
        private const val A_VDOWN = "dev.korokmote.VDOWN"
        private const val A_VUP = "dev.korokmote.VUP"
        private const val A_STOP = "dev.korokmote.STOP"

        @Volatile private var running = false

        fun start(ctx: Context) {
            if (running) return
            runCatching {
                ctx.startForegroundService(Intent(ctx, ConnectionService::class.java))
                running = true
            }
        }

        fun stop(ctx: Context) {
            if (!running) return
            running = false
            ctx.stopService(Intent(ctx, ConnectionService::class.java))
        }
    }
}
