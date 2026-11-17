package dev.korokmote.app

import dev.korokmote.app.net.KorokConnection
import dev.korokmote.app.net.Pairing
import dev.korokmote.app.protocol.AudioEvent
import dev.korokmote.app.protocol.DeviceInfo
import dev.korokmote.app.protocol.Phrase
import dev.korokmote.app.protocol.Protocol
import dev.korokmote.app.protocol.ProtocolException
import dev.korokmote.app.protocol.RemoteSummary
import dev.korokmote.app.ui.Msg
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Assume.assumeTrue
import org.junit.Test
import java.util.UUID

/**
 * End-to-end: the app's own networking code against a live Go daemon.
 * Run with: ./gradlew test -Pkorok.e2e=127.0.0.1:47810:<fingerprint>:<short code>
 */
class LiveDaemonTest {
    @Test
    fun pairConnectAndControl() = runBlocking {
        val spec = System.getProperty("korok.e2e").orEmpty()
        assumeTrue("set -Pkorok.e2e to run", spec.isNotEmpty())
        val (host, port, fp, code) = spec.split(":")
        Phrase.words = javaClass.classLoader!!.getResourceAsStream("wordlist.txt")!!.bufferedReader().readLines().filter { it.isNotBlank() }
        val device = DeviceInfo(UUID.randomUUID().toString(), "jvm-test", "android", "test")

        // Wrong pin must be rejected before anything is sent.
        try {
            Pairing.pair(listOf(host), port.toInt(), "0".repeat(64), code, device)
            fail("pin mismatch accepted")
        } catch (_: Exception) {
        }

        val r = Pairing.pair(listOf(host), port.toInt(), fp, code, device)
        assertEquals(fp, r.fingerprint)
        assertEquals(Phrase.derive(fp, device.id), r.response.phrase)

        val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        val conn = KorokConnection(scope, host, port.toInt(), fp, device.id, r.response.token, "test") { _, _, _ -> }
        val hello = conn.open()
        assertTrue(hello.caps.contains("input"))

        conn.request("key", Msg.key("ENTER"))
        conn.request("combo", Msg.combo(listOf("CTRL", "C")))
        conn.request("text", Msg.text("hello from kotlin"))
        repeat(3) { conn.pointer(10, -4) }
        conn.scroll(0, -120)
        conn.button(1, true)
        conn.button(1, false)
        conn.ping()
        withTimeout(3000) { conn.events.first { it.t == "pong" } }

        val a = Protocol.json.decodeFromJsonElement(AudioEvent.serializer(), conn.request("audio.set", Msg.volumeSet(0.33)))
        assertEquals(0.33, a.level, 0.001)

        val list = conn.request("remotes.list") as JsonObject
        val remotes = Protocol.json.decodeFromJsonElement(ListSerializer(RemoteSummary.serializer()), list["remotes"]!!)
        assertTrue(remotes.any { it.id == "vlc" })
        val layout = conn.request("remotes.get", Msg.id("vlc")) as JsonObject
        assertTrue(layout.containsKey("layout"))
        conn.request("remotes.act", Msg.act("youtube", "search", mapOf("query" to "lofi")))

        try {
            conn.request("fs.list", Msg.path("/definitely/not/allowed"))
            fail("path outside roots accepted")
        } catch (e: ProtocolException) {
            assertTrue(e.code == "forbidden" || e.code == "not_found")
        }
        conn.close()
    }
}
