package dev.korokmote.app

import dev.korokmote.app.protocol.Envelope
import dev.korokmote.app.protocol.Frames
import dev.korokmote.app.protocol.Hello
import dev.korokmote.app.protocol.HelloReply
import dev.korokmote.app.protocol.Phrase
import dev.korokmote.app.protocol.Protocol
import dev.korokmote.app.protocol.toHex
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

/** Uses the same golden vectors as the Go daemon (protocol/vectors). */
class ProtocolVectorsTest {
    private fun res(name: String): String =
        javaClass.classLoader!!.getResourceAsStream(name)!!.bufferedReader().readText()

    private fun hexToBytes(h: String) = ByteArray(h.length / 2) { h.substring(it * 2, it * 2 + 2).toInt(16).toByte() }

    @Test
    fun binaryFrames() {
        val frames = Json.parseToJsonElement(res("vectors/binary.json")).jsonObject["frames"]!!.jsonArray
        for (f in frames) {
            val o = f.jsonObject
            val op = o["op"]!!.jsonPrimitive.int
            val hex = o["hex"]!!.jsonPrimitive.content
            val enc = when (op) {
                1, 2 -> Frames.rel(op.toByte(), o["x"]!!.jsonPrimitive.int, o["y"]!!.jsonPrimitive.int, o["seq"]!!.jsonPrimitive.int)
                3 -> Frames.abs(o["x"]!!.jsonPrimitive.int, o["y"]!!.jsonPrimitive.int, o["seq"]!!.jsonPrimitive.int)
                else -> Frames.button(o["button"]!!.jsonPrimitive.int, o["down"]!!.jsonPrimitive.boolean)
            }
            assertEquals(o["name"]!!.jsonPrimitive.content, hex, enc.toHex())
            val dec = Frames.decode(hexToBytes(hex))
            assertNotNull(dec)
            if (op != 4) {
                assertEquals(o["x"]!!.jsonPrimitive.int, dec!!.x)
                assertEquals(o["y"]!!.jsonPrimitive.int, dec.y)
            }
        }
        assertNull(Frames.decode(byteArrayOf(0x7F, 1, 2)))
    }

    @Test
    fun phrase() {
        val words = res("wordlist.txt").lines().map { it.trim() }.filter { it.isNotEmpty() }
        assertEquals(256, words.size)
        val cases = Json.parseToJsonElement(res("vectors/phrase.json")).jsonObject["cases"]!!.jsonArray
        for (c in cases) {
            val o = c.jsonObject
            assertEquals(
                o["phrase"]!!.jsonPrimitive.content,
                Phrase.derive(o["fp"]!!.jsonPrimitive.content, o["device"]!!.jsonPrimitive.content, words),
            )
        }
    }

    @Test
    fun messages() {
        val msgs = Json.parseToJsonElement(res("vectors/messages.json")).jsonObject["messages"] as JsonArray
        for (m in msgs) {
            val env = Protocol.json.decodeFromJsonElement(Envelope.serializer(), m)
            when (env.t) {
                "hello" -> assertEquals(1, Protocol.json.decodeFromJsonElement(Hello.serializer(), env.d!!).v)
                "ok" -> assertEquals("DESKTOP-JOSE", Protocol.json.decodeFromJsonElement(HelloReply.serializer(), env.d!!).daemon.name)
            }
        }
        // Envelope encoding round-trips.
        val s = Protocol.envelope("key", 5, JsonObject(mapOf("k" to kotlinx.serialization.json.JsonPrimitive("A"))))
        val back = Protocol.json.decodeFromString(Envelope.serializer(), s)
        assertEquals("key", back.t)
        assertEquals(5L, back.id)
    }
}
