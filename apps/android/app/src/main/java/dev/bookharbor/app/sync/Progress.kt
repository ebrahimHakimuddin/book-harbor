package dev.bookharbor.app.sync

import org.json.JSONObject

/** A reopen location. EPUB uses a CFI string in [value]; PDF uses a 1-based [page]. */
data class Locator(val kind: String, val value: String = "", val page: Int = 0) {
    companion object {
        const val EPUB = "epub-cfi"
        const val PDF = "pdf-page"
        fun epub(cfi: String) = Locator(EPUB, value = cfi)
        fun pdf(page: Int) = Locator(PDF, page = page)
    }
}

/** An immutable reading-progress event: what the outbox stores and the server acknowledges. */
data class ProgressEvent(
    val eventId: String,
    val deviceId: String,
    val bookId: String,
    val editionId: String,
    /** RFC 3339 UTC instant, exactly as first written. */
    val occurredAt: String,
    val locator: Locator,
    val percentage: Double,
)

/** The canonical position the server returns for a book. */
data class ServerProgress(
    val revision: Long,
    val eventId: String,
    val deviceId: String,
    val bookId: String,
    val editionId: String,
    val occurredAt: String,
    val locator: Locator,
    val percentage: Double,
)

data class Acknowledgement(val eventId: String, val revision: Long, val disposition: String, val duplicate: Boolean)

data class SyncResponse(val cursor: Long, val hasMore: Boolean, val acknowledgements: List<Acknowledgement>, val progress: List<ServerProgress>)

/** What this device believes the position of a book to be. */
data class LocalPosition(
    val bookId: String,
    val editionId: String,
    val locator: Locator,
    val percentage: Double,
    val occurredAt: String,
    val eventId: String,
)

/** A rejected server request that will never succeed as written (a 4xx other than auth/rate limits). */
class PermanentSyncError(val status: Int, message: String) : Exception(message)

object ProgressCodec {
    fun encodeRequest(cursor: Long, changes: List<ProgressEvent>): String = JSONObject().apply {
        put("cursor", cursor)
        put("changes", org.json.JSONArray().also { array -> changes.forEach { array.put(encode(it)) } })
    }.toString()

    private fun encode(event: ProgressEvent) = JSONObject().apply {
        put("eventId", event.eventId)
        put("deviceId", event.deviceId)
        put("bookId", event.bookId)
        put("editionId", event.editionId)
        put("occurredAt", event.occurredAt)
        put("locator", JSONObject().apply {
            put("kind", event.locator.kind)
            if (event.locator.kind == Locator.EPUB) put("value", event.locator.value) else put("page", event.locator.page)
        })
        put("percentage", event.percentage)
    }

    fun decodeResponse(json: String): SyncResponse {
        val root = JSONObject(json)
        val acks = root.optJSONArray("acknowledgements") ?: org.json.JSONArray()
        val progress = root.optJSONArray("progress") ?: org.json.JSONArray()
        return SyncResponse(
            cursor = root.getLong("cursor"),
            hasMore = root.optBoolean("hasMore"),
            acknowledgements = (0 until acks.length()).map {
                val a = acks.getJSONObject(it)
                Acknowledgement(a.getString("eventId"), a.optLong("revision"), a.optString("disposition"), a.optBoolean("duplicate"))
            },
            progress = (0 until progress.length()).map {
                val p = progress.getJSONObject(it)
                val l = p.getJSONObject("locator")
                ServerProgress(
                    p.getLong("revision"), p.getString("eventId"), p.optString("deviceId"), p.getString("bookId"), p.getString("editionId"),
                    p.getString("occurredAt"), Locator(l.getString("kind"), l.optString("value"), l.optInt("page")), p.getDouble("percentage"),
                )
            },
        )
    }
}
