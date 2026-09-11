package dev.bookharbor.app.sync

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

/** [ProgressStore] on the framework SQLite: no extra dependency, real transactions. */
class SqliteProgressStore(context: Context) : SQLiteOpenHelper(context.applicationContext, "progress.db", null, 1), ProgressStore {
    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """CREATE TABLE position (
                book_id TEXT PRIMARY KEY, edition_id TEXT NOT NULL, locator_kind TEXT NOT NULL,
                locator_value TEXT NOT NULL, locator_page INTEGER NOT NULL, percentage REAL NOT NULL,
                occurred_at TEXT NOT NULL, event_id TEXT NOT NULL)""",
        )
        db.execSQL(
            """CREATE TABLE outbox (
                seq INTEGER PRIMARY KEY AUTOINCREMENT, event_id TEXT NOT NULL UNIQUE, device_id TEXT NOT NULL,
                book_id TEXT NOT NULL, edition_id TEXT NOT NULL, occurred_at TEXT NOT NULL,
                locator_kind TEXT NOT NULL, locator_value TEXT NOT NULL, locator_page INTEGER NOT NULL,
                percentage REAL NOT NULL, status TEXT NOT NULL DEFAULT 'pending', reason TEXT NOT NULL DEFAULT '')""",
        )
        db.execSQL("CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)")
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) = Unit

    override fun record(event: ProgressEvent) {
        val db = writableDatabase
        db.beginTransaction()
        try {
            putPosition(db, event.bookId, event.editionId, event.locator, event.percentage, event.occurredAt, event.eventId)
            db.insertWithOnConflict("outbox", null, ContentValues().apply {
                put("event_id", event.eventId); put("device_id", event.deviceId); put("book_id", event.bookId)
                put("edition_id", event.editionId); put("occurred_at", event.occurredAt)
                put("locator_kind", event.locator.kind); put("locator_value", event.locator.value)
                put("locator_page", event.locator.page); put("percentage", event.percentage)
            }, SQLiteDatabase.CONFLICT_IGNORE)
            db.setTransactionSuccessful()
        } finally {
            db.endTransaction()
        }
    }

    override fun position(bookId: String): LocalPosition? =
        readableDatabase.rawQuery("SELECT $POSITION_COLUMNS FROM position WHERE book_id = ?", arrayOf(bookId)).use { if (it.moveToFirst()) readPosition(it) else null }

    override fun positions(): List<LocalPosition> =
        readableDatabase.rawQuery("SELECT $POSITION_COLUMNS FROM position", null).use { cursor -> generateSequence { if (cursor.moveToNext()) readPosition(cursor) else null }.toList() }

    override fun pending(limit: Int): List<ProgressEvent> = readableDatabase.rawQuery(
        "SELECT event_id, device_id, book_id, edition_id, occurred_at, locator_kind, locator_value, locator_page, percentage FROM outbox WHERE status = 'pending' ORDER BY seq LIMIT ?",
        arrayOf(limit.toString()),
    ).use { c ->
        generateSequence {
            if (!c.moveToNext()) null
            else ProgressEvent(c.getString(0), c.getString(1), c.getString(2), c.getString(3), c.getString(4), Locator(c.getString(5), c.getString(6), c.getInt(7)), c.getDouble(8))
        }.toList()
    }

    override fun pendingCount() = count("SELECT COUNT(*) FROM outbox WHERE status = 'pending'")
    override fun rejectedCount() = count("SELECT COUNT(*) FROM outbox WHERE status = 'rejected'")

    override fun cursor(): Long =
        readableDatabase.rawQuery("SELECT value FROM meta WHERE key = 'cursor'", null).use { if (it.moveToFirst()) it.getString(0).toLong() else 0L }

    override fun applySync(acknowledged: Set<String>, server: List<ServerProgress>, cursor: Long) {
        val db = writableDatabase
        db.beginTransaction()
        try {
            acknowledged.forEach { db.delete("outbox", "event_id = ?", arrayOf(it)) }
            for (incoming in server) {
                val local = db.rawQuery("SELECT $POSITION_COLUMNS FROM position WHERE book_id = ?", arrayOf(incoming.bookId)).use { if (it.moveToFirst()) readPosition(it) else null }
                val unsent = unsentCount(db, incoming.bookId) > 0
                if (shouldAdopt(local, incoming, unsent)) {
                    putPosition(db, incoming.bookId, incoming.editionId, incoming.locator, incoming.percentage, incoming.occurredAt, incoming.eventId)
                }
            }
            db.insertWithOnConflict("meta", null, ContentValues().apply { put("key", "cursor"); put("value", cursor.toString()) }, SQLiteDatabase.CONFLICT_REPLACE)
            db.setTransactionSuccessful()
        } finally {
            db.endTransaction()
        }
    }

    override fun reject(eventId: String, reason: String) {
        writableDatabase.update("outbox", ContentValues().apply { put("status", "rejected"); put("reason", reason.take(300)) }, "event_id = ?", arrayOf(eventId))
    }

    private fun unsentCount(db: SQLiteDatabase, bookId: String): Long =
        db.rawQuery("SELECT COUNT(*) FROM outbox WHERE book_id = ? AND status = 'pending'", arrayOf(bookId)).use { it.moveToFirst(); it.getLong(0) }

    private fun count(sql: String) = readableDatabase.rawQuery(sql, null).use { it.moveToFirst(); it.getInt(0) }

    private fun putPosition(db: SQLiteDatabase, bookId: String, editionId: String, locator: Locator, percentage: Double, occurredAt: String, eventId: String) {
        db.insertWithOnConflict("position", null, ContentValues().apply {
            put("book_id", bookId); put("edition_id", editionId); put("locator_kind", locator.kind)
            put("locator_value", locator.value); put("locator_page", locator.page); put("percentage", percentage)
            put("occurred_at", occurredAt); put("event_id", eventId)
        }, SQLiteDatabase.CONFLICT_REPLACE)
    }

    private fun readPosition(c: android.database.Cursor) =
        LocalPosition(c.getString(0), c.getString(1), Locator(c.getString(2), c.getString(3), c.getInt(4)), c.getDouble(5), c.getString(6), c.getString(7))

    private companion object {
        const val POSITION_COLUMNS = "book_id, edition_id, locator_kind, locator_value, locator_page, percentage, occurred_at, event_id"
    }
}

/**
 * The reconciliation rule from docs/offline-sync.md: take the server position only when it is
 * newer than the local one and no unsent local event still represents this book.
 */
fun shouldAdopt(local: LocalPosition?, incoming: ServerProgress, hasUnsentLocalEvent: Boolean): Boolean {
    if (hasUnsentLocalEvent) return false
    if (local == null) return true
    return java.time.Instant.parse(incoming.occurredAt).isAfter(java.time.Instant.parse(local.occurredAt))
}
