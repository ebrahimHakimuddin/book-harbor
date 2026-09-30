package dev.bookharbor.app.reader

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.luminance
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.IOException

class ReaderSurfaceTest {
    @Test fun customColorAlwaysChoosesReadableInk() {
        for (red in 0..255 step 17) for (green in 0..255 step 17) for (blue in 0..255 step 17) {
            val background = Color(red, green, blue)
            val foreground = readablePageTextColor(background)
            val high = maxOf(background.luminance(), foreground.luminance())
            val low = minOf(background.luminance(), foreground.luminance())
            assertTrue("Unreadable ink on $background", (high + 0.05f) / (low + 0.05f) >= 4.5f)
        }
    }

    @Test fun imageInputIsBoundedBeforeDecoding() {
        val bytes = byteArrayOf(1, 2, 3)
        assertArrayEquals(bytes, ReaderBackgrounds.readImageBytes(ByteArrayInputStream(bytes)))
        assertThrows(IOException::class.java) {
            ReaderBackgrounds.readImageBytes(ByteArrayInputStream(ByteArray(16 * 1024 * 1024 + 1)))
        }
    }
}
