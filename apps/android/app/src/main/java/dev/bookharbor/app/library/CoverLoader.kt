package dev.bookharbor.app.library

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.LruCache
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import java.io.File
import java.security.MessageDigest

/**
 * Loads book covers. Covers uploaded to the server need the session token; covers from other
 * sites are fetched without it so the token never leaves the user's own server. Decoded
 * images live in a small memory cache backed by files in the app cache directory.
 */
class CoverLoader(private val api: ApiClient, cacheDir: File) {
    private val directory = File(cacheDir, "covers").apply { mkdirs() }
    private val memory = object : LruCache<String, ImageBitmap>(24 * 1024 * 1024) {
        override fun sizeOf(key: String, value: ImageBitmap) = value.width * value.height * 4
    }

    /** The decoded cover if it is already in memory; cheap enough to call while composing. */
    fun peek(book: Book): ImageBitmap? = if (book.coverUrl.isBlank()) null else memory.get(hash(book.coverUrl + "|" + book.updatedAt))

    /** Blocking; call from Dispatchers.IO. Returns null when there is no cover or it cannot be fetched. */
    fun load(book: Book): ImageBitmap? {
        if (book.coverUrl.isBlank()) return null
        val key = hash(book.coverUrl + "|" + book.updatedAt)
        memory.get(key)?.let { return it }
        val file = File(directory, key)
        val bytes = when {
            file.isFile -> file.readBytes()
            else -> runCatching { fetch(book.coverUrl) }.getOrNull()?.also { runCatching { file.writeBytes(it) } }
        } ?: return null
        // prepareToDraw uploads the pixels to the GPU now, on this IO thread, rather than on the
        // first frame that draws the cover -- which is what made scrolling stutter as covers arrived.
        return decode(bytes)?.also { it.prepareToDraw() }?.asImageBitmap()?.also { memory.put(key, it) }
    }

    /** The cover already on disk, without touching the network (for the home-screen widget). */
    fun cached(book: Book): Bitmap? {
        if (book.coverUrl.isBlank()) return null
        val file = File(directory, hash(book.coverUrl + "|" + book.updatedAt))
        return if (file.isFile) runCatching { decode(file.readBytes()) }.getOrNull() else null
    }

    private fun fetch(url: String): ByteArray =
        if (url.startsWith("/")) api.authorizedBytes(url) else api.requestBytes(url)

    private fun decode(bytes: ByteArray): Bitmap? {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        if (bounds.outWidth <= 0) return null
        var sample = 1
        while (bounds.outWidth / (sample * 2) >= TARGET_WIDTH) sample *= 2
        return BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
    }

    private val hashes = LruCache<String, String>(512)

    private fun hash(value: String) = hashes.get(value) ?: sha(value).also { hashes.put(value, it) }

    private fun sha(value: String) = MessageDigest.getInstance("SHA-256").digest(value.toByteArray()).joinToString("") { "%02x".format(it) }.take(40)

    private companion object { const val TARGET_WIDTH = 360 }
}
