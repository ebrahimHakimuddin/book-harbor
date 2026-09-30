package dev.bookharbor.app.reader

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.io.InputStream

/** A single private, bounded image shared by the device-local reader profile. */
internal object ReaderBackgrounds {
    private const val maxInputBytes = 16 * 1024 * 1024
    private const val maxDimension = 2048

    fun file(context: Context): File = File(context.filesDir, "reader/page-background.jpg")

    fun import(context: Context, uri: Uri): Long {
        val bytes = context.contentResolver.openInputStream(uri)?.use(::readImageBytes)
            ?: throw IOException("Image could not be opened")
        return install(context, bytes)
    }

    internal fun readImageBytes(input: InputStream): ByteArray {
        val output = ByteArrayOutputStream()
        val buffer = ByteArray(16 * 1024)
        while (true) {
            val count = input.read(buffer)
            if (count < 0) break
            if (output.size() + count > maxInputBytes) throw IOException("Image exceeds 16 MiB")
            output.write(buffer, 0, count)
        }
        return output.toByteArray()
    }

    private fun install(context: Context, bytes: ByteArray): Long {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) throw IOException("Invalid image")
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / sample > maxDimension) sample *= 2
        val bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
            ?: throw IOException("Image could not be decoded")
        val destination = file(context)
        destination.parentFile?.mkdirs()
        val temporary = File(destination.parentFile, "page-background.tmp")
        try {
            temporary.outputStream().use { if (!bitmap.compress(Bitmap.CompressFormat.JPEG, 84, it)) throw IOException("Image could not be saved") }
            if (!temporary.renameTo(destination)) throw IOException("Image could not be installed")
        } finally {
            bitmap.recycle()
            temporary.delete()
        }
        return System.currentTimeMillis().coerceAtLeast(1)
    }

    fun load(context: Context): ImageBitmap? = runCatching {
        val source = file(context)
        if (!source.isFile || source.length() > maxInputBytes) return@runCatching null
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeFile(source.path, bounds)
        if (bounds.outWidth <= 0 || bounds.outHeight <= 0) return@runCatching null
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / sample > maxDimension) sample *= 2
        BitmapFactory.decodeFile(source.path, BitmapFactory.Options().apply { inSampleSize = sample })?.asImageBitmap()
    }.getOrNull()

    fun remove(context: Context) { file(context).delete() }
}
