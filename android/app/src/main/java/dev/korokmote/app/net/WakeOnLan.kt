package dev.korokmote.app.net

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress

object WakeOnLan {
    fun parseMac(mac: String): ByteArray? {
        val parts = mac.split(':', '-')
        if (parts.size != 6) return null
        return runCatching { ByteArray(6) { parts[it].toInt(16).toByte() } }.getOrNull()
    }

    fun packet(mac: ByteArray): ByteArray = ByteArray(6) { 0xFF.toByte() } + (0 until 16).flatMap { mac.toList() }

    /** Sends the magic packet by broadcast to ports 9 and 7, plus the last known address. */
    suspend fun send(mac: String, lastAddress: String?): Boolean = withContext(Dispatchers.IO) {
        val bytes = parseMac(mac) ?: return@withContext false
        val data = packet(bytes)
        val targets = mutableListOf("255.255.255.255")
        lastAddress?.let { addr ->
            val p = addr.split('.')
            if (p.size == 4) targets += "${p[0]}.${p[1]}.${p[2]}.255"
        }
        var ok = false
        DatagramSocket().use { s ->
            s.broadcast = true
            for (t in targets) for (port in intArrayOf(9, 7)) {
                runCatching {
                    s.send(DatagramPacket(data, data.size, InetAddress.getByName(t), port))
                    ok = true
                }
            }
        }
        ok
    }
}
