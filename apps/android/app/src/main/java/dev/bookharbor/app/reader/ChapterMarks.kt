package dev.bookharbor.app.reader

import android.content.SharedPreferences
import org.json.JSONObject

/**
 * Which chapters of each book the reader has marked read or unread, on this device. A mark is
 * explicit; chapters without one fall back to "read if it's before where the reader is now"
 * (see [chapterReadStates]). The reader also marks a chapter read on finishing it.
 */
class ChapterMarks(private val preferences: SharedPreferences) {
    fun get(bookId: String): Map<Int, Boolean> = runCatching {
        val json = JSONObject(preferences.getString(PREFIX + bookId, "{}") ?: "{}")
        json.keys().asSequence().associate { it.toInt() to json.getBoolean(it) }
    }.getOrDefault(emptyMap())

    fun set(bookId: String, chapters: Collection<Int>, read: Boolean) {
        val next = get(bookId) + chapters.associateWith { read }
        preferences.edit().putString(PREFIX + bookId, JSONObject(next.mapKeys { it.key.toString() }).toString()).apply()
    }

    /** Chapters 0..[index] become read; later ones keep whatever they were. */
    fun markReadUpTo(bookId: String, index: Int) = set(bookId, (0..index).toList(), true)

    fun clear(bookId: String) { preferences.edit().remove(PREFIX + bookId).apply() }

    private companion object { const val PREFIX = "chapters.read." }
}

/**
 * Read state for each of [count] chapters: an explicit mark wins; otherwise a chapter counts as
 * read when it comes before [currentChapter], the one the reader is in (null if never opened).
 */
fun chapterReadStates(count: Int, explicit: Map<Int, Boolean>, currentChapter: Int?): List<Boolean> =
    List(count) { index -> explicit[index] ?: (currentChapter != null && index < currentChapter) }
