package dev.korokmote.app.net

import dev.korokmote.app.protocol.sha256Hex
import okhttp3.OkHttpClient
import java.security.SecureRandom
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager

/**
 * Trusts exactly one certificate, identified by the SHA-256 of its DER encoding.
 * With pin == null it accepts any certificate and records it (manual pairing only).
 */
class PinningTrustManager(private val pin: String?) : X509TrustManager {
    @Volatile
    var seen: String? = null
        private set

    override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
        val leaf = chain?.firstOrNull() ?: throw CertificateException("no certificate")
        val fp = sha256Hex(leaf.encoded)
        seen = fp
        if (pin != null && !pin.equals(fp, ignoreCase = true)) {
            throw CertificateException("certificate does not match the paired PC")
        }
    }

    override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) =
        throw CertificateException("client certificates not supported")

    override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
}

object Http {
    private val base: OkHttpClient by lazy {
        OkHttpClient.Builder()
            .connectTimeout(4, TimeUnit.SECONDS)
            .readTimeout(0, TimeUnit.SECONDS)
            .writeTimeout(5, TimeUnit.SECONDS)
            .retryOnConnectionFailure(false)
            .proxy(java.net.Proxy.NO_PROXY)
            .build()
    }

    /** A client that only talks to the pinned certificate; the identity is the pin, not the hostname. */
    fun pinned(pin: String?): Pair<OkHttpClient, PinningTrustManager> {
        val tm = PinningTrustManager(pin)
        val ctx = SSLContext.getInstance("TLSv1.3")
        ctx.init(null, arrayOf(tm), SecureRandom())
        val client = base.newBuilder()
            .sslSocketFactory(ctx.socketFactory, tm)
            .hostnameVerifier { _, _ -> true }
            .build()
        return client to tm
    }
}
