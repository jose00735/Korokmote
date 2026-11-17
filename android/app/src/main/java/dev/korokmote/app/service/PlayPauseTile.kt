package dev.korokmote.app.service

import android.app.PendingIntent
import android.content.Intent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import dev.korokmote.app.KorokApp
import dev.korokmote.app.MainActivity
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** Quick Settings tile: play/pause on the PC, or open the app when not connected. */
class PlayPauseTile : TileService() {
    override fun onStartListening() {
        val online = (application as KorokApp).connection.online
        qsTile?.apply {
            state = if (online) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
            updateTile()
        }
    }

    override fun onClick() {
        val c = (application as KorokApp).connection
        if (c.online) {
            c.fire("media", buildJsonObject { put("a", "playpause") })
            return
        }
        val intent = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        if (Build.VERSION.SDK_INT >= 34) {
            startActivityAndCollapse(PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE))
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(intent)
        }
    }
}
