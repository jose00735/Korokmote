package dev.korokmote.app.ui

import android.annotation.SuppressLint
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.LocalLifecycleOwner
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer
import dev.korokmote.app.design.LocalMatrix
import java.util.concurrent.Executors

/** Camera preview that decodes QR codes with ZXing and reports the first korokmote:// payload. */
@Composable
fun QrScanner(onResult: (String) -> Unit, modifier: Modifier = Modifier, locked: Boolean = false) {
    val ctx = LocalContext.current
    val owner = LocalLifecycleOwner.current
    val cfg = LocalMatrix.current
    val c = cfg.colors
    val result by rememberUpdatedState(onResult)
    val executor = remember { Executors.newSingleThreadExecutor() }
    val previewView = remember { PreviewView(ctx).apply { scaleType = PreviewView.ScaleType.FILL_CENTER } }
    DisposableEffect(owner) {
        val future = ProcessCameraProvider.getInstance(ctx)
        var provider: ProcessCameraProvider? = null
        future.addListener({
            val p = future.get()
            provider = p
            val preview = Preview.Builder().build().also { it.surfaceProvider = previewView.surfaceProvider }
            val reader = MultiFormatReader().apply { setHints(mapOf(DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE))) }
            val analysis = ImageAnalysis.Builder().setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST).build()
            var done = false
            analysis.setAnalyzer(executor) { img ->
                if (!done) {
                    decode(reader, img)?.let { text ->
                        if (text.startsWith("korokmote://")) {
                            done = true
                            previewView.post { result(text) }
                        }
                    }
                }
                img.close()
            }
            runCatching {
                p.unbindAll()
                p.bindToLifecycle(owner, CameraSelector.DEFAULT_BACK_CAMERA, preview, analysis)
            }
        }, ContextCompat.getMainExecutor(ctx))
        onDispose {
            provider?.unbindAll()
            executor.shutdown()
        }
    }
    // Corner brackets that "seek" inward, then lock (DESIGN.md 5.7).
    val seek = if (cfg.animated && !locked) {
        rememberInfiniteTransition(label = "seek").animateFloat(0f, 1f, infiniteRepeatable(tween(900), RepeatMode.Reverse), label = "s").value
    } else {
        1f
    }
    Box(modifier) {
        AndroidView({ previewView }, Modifier.fillMaxSize())
        Box(
            Modifier.fillMaxSize().drawWithContent {
                drawContent()
                val side = size.minDimension * (0.72f - 0.06f * seek)
                val l = (size.width - side) / 2
                val t = (size.height - side) / 2
                val arm = side * 0.16f
                val w = 4.dp2(density)
                val col = if (locked) c.head else c.bright
                for ((x, y, dx, dy) in listOf(
                    listOf(l, t, 1f, 1f), listOf(l + side, t, -1f, 1f),
                    listOf(l, t + side, 1f, -1f), listOf(l + side, t + side, -1f, -1f),
                )) {
                    drawLine(col, Offset(x, y), Offset(x + arm * dx, y), w)
                    drawLine(col, Offset(x, y), Offset(x, y + arm * dy), w)
                }
            },
        )
    }
}

private fun Int.dp2(density: Float) = this * density

@SuppressLint("UnsafeOptInUsageError")
private fun decode(reader: MultiFormatReader, img: ImageProxy): String? {
    val plane = img.planes.firstOrNull() ?: return null
    val buf = plane.buffer
    val data = ByteArray(buf.remaining()).also { buf.get(it) }
    val w = plane.rowStride
    val h = img.height
    return runCatching {
        val src = PlanarYUVLuminanceSource(data, w, h, 0, 0, img.width, h, false)
        reader.decodeWithState(BinaryBitmap(HybridBinarizer(src))).text
    }.getOrNull() ?: runCatching {
        val src = PlanarYUVLuminanceSource(data, w, h, 0, 0, img.width, h, false).invert()
        reader.decodeWithState(BinaryBitmap(HybridBinarizer(src))).text
    }.getOrNull().also { reader.reset() }
}
