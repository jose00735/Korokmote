package dev.korokmote.app

import dev.korokmote.app.net.WakeOnLan
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class WakeOnLanTest {
    @Test
    fun magicPacket() {
        val mac = WakeOnLan.parseMac("aa:bb:cc:dd:ee:ff")!!
        val p = WakeOnLan.packet(mac)
        assertEquals(102, p.size)
        assertArrayEquals(ByteArray(6) { 0xFF.toByte() }, p.copyOfRange(0, 6))
        assertArrayEquals(mac, p.copyOfRange(96, 102))
        assertNull(WakeOnLan.parseMac("nope"))
    }
}
