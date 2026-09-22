package dev.bookharbor.app.reader

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import dev.bookharbor.app.reader.epub.EpubPosition
import dev.bookharbor.app.sync.Locator
import java.time.Instant
import java.util.UUID

enum class AnnotationKind { Bookmark, Highlight }

/**
 * A bookmark or highlight. [locator] uses the same shape as reading progress (EPUB CFI of the
 * block, or PDF page); a highlighted passage starts at the CFI's character offset and ends at
 * [endOffset] within that block, where 0 means the whole block. [label] is the chapter title or
 * page shown in lists. [syncId] is the ID shared with the server and other devices.
 */
data class Annotation(
    val id: Long = 0,
    val bookId: String,
    val kind: AnnotationKind,
    val locator: Locator,
    val label: String,
    val excerpt: String = "",
    val note: String = "",
    val createdAt: String,
    val syncId: String = "",
    val endOffset: Int = 0,
    val updatedAt: String = createdAt,
    val deleted: Boolean = false,
)

/**
 * Annotations on this device, plus the queue of edits the server hasn't acknowledged. Every
 * edit marks its row dirty and bumps updated_at; a delete keeps a tombstone until synced, so
 * other devices learn of it. The server keeps whichever edit is newest.
 */
class AnnotationStore(context: Context, private val onChanged: () -> Unit = {}) :
    SQLiteOpenHelper(context.applicationContext, "annotations.db", null, 2) {
    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """CREATE TABLE annotation (
                id INTEGER PRIMARY KEY AUTOINCREMENT, book_id TEXT NOT NULL, kind TEXT NOT NULL,
                locator_kind TEXT NOT NULL, locator_value TEXT NOT NULL, locator_page INTEGER NOT NULL,
                label TEXT NOT NULL, excerpt TEXT NOT NULL, note TEXT NOT NULL, created_at TEXT NOT NULL)""",
        )
        db.execSQL("CREATE INDEX annotation_book ON annotation(book_id)")
        onUpgrade(db, 1, 2)
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        if (oldVersion < 2) {
            db.execSQL("ALTER TABLE annotation ADD COLUMN sync_id TEXT NOT NULL DEFAULT ''")
            db.execSQL("ALTER TABLE annotation ADD COLUMN end_offset INTEGER NOT NULL DEFAULT 0")
            db.execSQL("ALTER TABLE annotation ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''")
            db.execSQL("ALTER TABLE annotation ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0")
            db.execSQL("ALTER TABLE annotation ADD COLUMN dirty INTEGER NOT NULL DEFAULT 1")
            // Rows made before sync existed get an ID and are queued for their first upload.
            db.execSQL("UPDATE annotation SET sync_id = 'ann_' || lower(hex(randomblob(16))), updated_at = created_at")
            db.execSQL("CREATE UNIQUE INDEX annotation_sync_id ON annotation(sync_id)")
            db.execSQL("CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
        }
    }

    fun forBook(bookId: String): List<Annotation> = select("WHERE book_id = ? AND deleted = 0 ORDER BY id", bookId)

    fun add(annotation: Annotation): Annotation {
        val saved = annotation.copy(syncId = annotation.syncId.ifEmpty { "ann_" + UUID.randomUUID().toString().replace("-", "") })
        val id = writableDatabase.insertOrThrow("annotation", null, values(saved, dirty = true))
        onChanged()
        return saved.copy(id = id)
    }

    fun setNote(id: Long, note: String) = edit(id, ContentValues().apply { put("note", note) })

    fun delete(id: Long) = edit(id, ContentValues().apply { put("deleted", 1) })

    private fun edit(id: Long, change: ContentValues) {
        change.put("updated_at", Instant.now().toString())
        change.put("dirty", 1)
        writableDatabase.update("annotation", change, "id = ?", arrayOf(id.toString()))
        onChanged()
    }

    /** Oldest unsent edits, tombstones included. */
    fun pending(limit: Int): List<Annotation> = select("WHERE dirty = 1 ORDER BY updated_at LIMIT $limit")

    fun pendingCount(): Int = readableDatabase.rawQuery("SELECT COUNT(*) FROM annotation WHERE dirty = 1", null).use { it.moveToFirst(); it.getInt(0) }

    fun cursor(): Long =
        readableDatabase.rawQuery("SELECT value FROM meta WHERE key = 'cursor'", null).use { if (it.moveToFirst()) it.getString(0).toLong() else 0L }

    /**
     * After a successful exchange, atomically: mark [sent] rows clean unless edited again
     * meanwhile, drop [rejected] ones from the queue, take [incoming] versions for rows with no
     * unsent local edit, and store [cursor].
     */
    fun applySync(sent: List<Annotation>, accepted: Set<String>, rejected: Set<String>, incoming: List<Annotation>, cursor: Long) {
        val db = writableDatabase
        db.beginTransaction()
        try {
            for (item in sent) {
                if (item.syncId in accepted || item.syncId in rejected) {
                    db.execSQL("UPDATE annotation SET dirty = 0 WHERE sync_id = ? AND updated_at = ?", arrayOf(item.syncId, item.updatedAt))
                }
            }
            for (item in incoming) {
                val localDirty = db.rawQuery("SELECT dirty FROM annotation WHERE sync_id = ?", arrayOf(item.syncId)).use { if (it.moveToFirst()) it.getInt(0) == 1 else null }
                when (localDirty) {
                    true -> Unit // our unsent edit goes up next; the server settles who wins
                    false -> db.update("annotation", values(item, dirty = false), "sync_id = ?", arrayOf(item.syncId))
                    null -> if (!item.deleted) db.insert("annotation", null, values(item, dirty = false))
                }
            }
            // Acknowledged tombstones have done their job.
            db.delete("annotation", "deleted = 1 AND dirty = 0", null)
            db.insertWithOnConflict("meta", null, ContentValues().apply { put("key", "cursor"); put("value", cursor.toString()) }, SQLiteDatabase.CONFLICT_REPLACE)
            db.setTransactionSuccessful()
        } finally {
            db.endTransaction()
        }
    }

    /** Forgets this account's annotations and sync state (signing out). */
    fun clear() {
        val db = writableDatabase
        db.beginTransaction()
        try {
            db.delete("annotation", null, null)
            db.delete("meta", null, null)
            db.setTransactionSuccessful()
        } finally {
            db.endTransaction()
        }
    }

    private fun select(where: String, vararg args: String): List<Annotation> = readableDatabase.rawQuery(
        "SELECT id, book_id, kind, locator_kind, locator_value, locator_page, label, excerpt, note, created_at, sync_id, end_offset, updated_at, deleted FROM annotation $where",
        args,
    ).use { c ->
        generateSequence {
            if (!c.moveToNext()) null
            else Annotation(
                c.getLong(0), c.getString(1), AnnotationKind.entries.firstOrNull { it.name == c.getString(2) } ?: AnnotationKind.Bookmark,
                Locator(c.getString(3), c.getString(4), c.getInt(5)), c.getString(6), c.getString(7), c.getString(8), c.getString(9),
                c.getString(10), c.getInt(11), c.getString(12), c.getInt(13) == 1,
            )
        }.toList()
    }

    private fun values(a: Annotation, dirty: Boolean) = ContentValues().apply {
        put("book_id", a.bookId); put("kind", a.kind.name)
        put("locator_kind", a.locator.kind); put("locator_value", a.locator.value); put("locator_page", a.locator.page)
        put("label", a.label); put("excerpt", a.excerpt); put("note", a.note); put("created_at", a.createdAt)
        put("sync_id", a.syncId); put("end_offset", a.endOffset); put("updated_at", a.updatedAt)
        put("deleted", if (a.deleted) 1 else 0); put("dirty", if (dirty) 1 else 0)
    }
}

/** Plain text for the share sheet: every bookmark, highlight, and note in reading order. */
fun exportAnnotations(bookTitle: String, annotations: List<Annotation>): String = buildString {
    append(bookTitle).append("\n")
    annotations.sortedWith(InReadingOrder).forEach { a ->
        append("\n")
        when (a.kind) {
            AnnotationKind.Bookmark -> append("Bookmark · ").append(a.label).append("\n")
            AnnotationKind.Highlight -> append("“").append(a.excerpt.trim()).append("”\n  — ").append(a.label).append("\n")
        }
        if (a.note.isNotBlank()) append("  Note: ").append(a.note.trim()).append("\n")
    }
}

val InReadingOrder = Comparator<Annotation> { a, b -> compareSteps(readingOrder(a.locator), readingOrder(b.locator)) }

/** Numeric sort key: PDF page, or EPUB chapter followed by the block's CFI steps. */
private fun readingOrder(locator: Locator): List<Int> =
    if (locator.kind == Locator.PDF) listOf(locator.page)
    else EpubPosition.parse(locator.value)?.let { listOf(it.chapterIndex) + it.path.split('/').mapNotNull(String::toIntOrNull) } ?: emptyList()

private fun compareSteps(a: List<Int>, b: List<Int>): Int {
    for (i in 0 until minOf(a.size, b.size)) if (a[i] != b[i]) return a[i].compareTo(b[i])
    return a.size.compareTo(b.size)
}

/** The characters a highlight covers in its block of [length] characters; whole block when it has no end. */
fun highlightRange(highlight: Annotation, length: Int): IntRange {
    val start = (EpubPosition.parse(highlight.locator.value)?.offset ?: 0).coerceIn(0, length)
    val end = if (highlight.endOffset > start) highlight.endOffset.coerceAtMost(length) else length
    return start until end
}

/** A search match: where it is and a short snippet around it. */
data class SearchHit(val locator: Locator, val label: String, val snippet: String)

/** Up to ~[radius] characters either side of the first match of [query] in [text], or null if absent. */
fun snippetAround(text: String, query: String, radius: Int = 60): String? {
    val at = text.indexOf(query, ignoreCase = true)
    if (at < 0) return null
    val start = (at - radius).coerceAtLeast(0)
    val end = (at + query.length + radius).coerceAtMost(text.length)
    return (if (start > 0) "…" else "") + text.substring(start, end).replace(Regex("\\s+"), " ").trim() + (if (end < text.length) "…" else "")
}

/** One open book's annotations as Compose state, over [AnnotationStore]. Writes are single small rows. */
class BookAnnotations(private val store: AnnotationStore, val bookId: String, val bookTitle: String) {
    var items by mutableStateOf(emptyList<Annotation>())
        private set

    /** Call off the main thread. */
    fun load() { items = store.forBook(bookId) }

    fun find(kind: AnnotationKind, locator: Locator): Annotation? = items.firstOrNull { it.kind == kind && it.locator == locator }

    fun add(kind: AnnotationKind, locator: Locator, label: String, excerpt: String = "", endOffset: Int = 0): Annotation =
        store.add(Annotation(bookId = bookId, kind = kind, locator = locator, label = label, excerpt = excerpt, endOffset = endOffset, createdAt = Instant.now().toString()))
            .also { items = items + it }

    fun toggleBookmark(locator: Locator, label: String) {
        find(AnnotationKind.Bookmark, locator)?.let(::delete) ?: add(AnnotationKind.Bookmark, locator, label)
    }

    fun delete(annotation: Annotation) {
        store.delete(annotation.id)
        items = items.filterNot { it.id == annotation.id }
    }

    fun setNote(annotation: Annotation, note: String) {
        store.setNote(annotation.id, note.trim())
        items = items.map { if (it.id == annotation.id) it.copy(note = note.trim()) else it }
    }
}

fun shareText(context: Context, text: String) {
    val send = Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, text)
    runCatching { context.startActivity(Intent.createChooser(send, null)) }
}
