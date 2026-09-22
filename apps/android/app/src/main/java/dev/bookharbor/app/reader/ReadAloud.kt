package dev.bookharbor.app.reader

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import java.util.Locale

/**
 * Reads a chapter's blocks aloud with the platform text-to-speech engine, one block per
 * utterance, so the reader can follow along and pause at a block boundary. [speaking] is the
 * block being read, or null when stopped.
 */
class ReadAloud(context: Context) {
    private val main = Handler(Looper.getMainLooper())
    private var blocks: List<String?> = emptyList()
    private var onFinished: () -> Unit = {}
    private var engine: TextToSpeech? = null

    var available by mutableStateOf(false)
        private set
    var speaking by mutableStateOf<Int?>(null)
        private set
    var paused by mutableStateOf(false)
        private set

    init {
        engine = TextToSpeech(context.applicationContext) { status ->
            main.post { available = status == TextToSpeech.SUCCESS }
        }
        engine?.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
            override fun onStart(utteranceId: String) { main.post { blockOf(utteranceId)?.let { speaking = it } } }
            override fun onDone(utteranceId: String) {
                // The last part of the last block: the chapter is read.
                main.post { if (utteranceId == lastUtterance && !paused) { speaking = null; onFinished() } }
            }
            @Deprecated("Deprecated in Java") override fun onError(utteranceId: String) { main.post { speaking = null } }
        })
    }

    private var lastUtterance = ""

    /** Starts reading [chapterBlocks] (null for images and rules, which are skipped) from [from]. */
    fun play(chapterBlocks: List<String?>, from: Int, language: Locale? = null, onChapterEnd: () -> Unit = {}) {
        val tts = engine ?: return
        blocks = chapterBlocks
        onFinished = onChapterEnd
        paused = false
        language?.let { tts.language = it }
        tts.stop()
        val limit = TextToSpeech.getMaxSpeechInputLength().coerceAtMost(3900)
        var queued = false
        for (index in from.coerceAtLeast(0) until blocks.size) {
            val text = blocks[index]?.takeIf { it.isNotBlank() } ?: continue
            chunks(text, limit).forEachIndexed { part, chunk ->
                val id = "$index:$part"
                tts.speak(chunk, if (queued) TextToSpeech.QUEUE_ADD else TextToSpeech.QUEUE_FLUSH, null, id)
                lastUtterance = id
                queued = true
            }
        }
        speaking = if (queued) from else null
    }

    /** Pausing stops at once; resuming starts again from the beginning of the block that was playing. */
    fun togglePause() {
        val block = speaking ?: return
        if (paused) play(blocks, block, onChapterEnd = onFinished)
        else { paused = true; engine?.stop() }
    }

    fun stop() {
        paused = false
        speaking = null
        engine?.stop()
    }

    fun shutdown() {
        engine?.stop()
        engine?.shutdown()
        engine = null
    }

    private fun blockOf(utteranceId: String) = utteranceId.substringBefore(':').toIntOrNull()
}

/** Splits [text] into pieces of at most [limit] characters, preferring sentence then word breaks. */
internal fun chunks(text: String, limit: Int): List<String> {
    if (text.length <= limit) return listOf(text)
    val parts = ArrayList<String>()
    var rest = text
    while (rest.length > limit) {
        val window = rest.substring(0, limit)
        val cut = listOf(". ", "! ", "? ", "; ", ", ", " ").firstNotNullOfOrNull { mark -> window.lastIndexOf(mark).takeIf { it > limit / 2 }?.plus(mark.length) } ?: limit
        parts += rest.substring(0, cut).trim()
        rest = rest.substring(cut)
    }
    if (rest.isNotBlank()) parts += rest.trim()
    return parts
}
