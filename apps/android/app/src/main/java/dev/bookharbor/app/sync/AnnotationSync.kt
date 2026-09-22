package dev.bookharbor.app.sync

import dev.bookharbor.app.library.ApiClient
import dev.bookharbor.app.reader.Annotation
import dev.bookharbor.app.reader.AnnotationKind
import dev.bookharbor.app.reader.AnnotationStore
import org.json.JSONArray
import org.json.JSONObject

data class AnnotationSyncResponse(
    val cursor: Long,
    val hasMore: Boolean,
    val accepted: Set<String>,
    val rejected: Set<String>,
    val annotations: List<Annotation>,
)

object AnnotationCodec {
    fun encodeRequest(cursor: Long, changes: List<Annotation>): String = JSONObject().apply {
        put("cursor", cursor)
        put("changes", JSONArray().also { array -> changes.forEach { array.put(encode(it)) } })
    }.toString()

    private fun encode(a: Annotation) = JSONObject().apply {
        put("id", a.syncId); put("bookId", a.bookId); put("kind", a.kind.name.lowercase())
        put("locator", JSONObject().apply { put("kind", a.locator.kind); put("value", a.locator.value); put("page", a.locator.page) })
        put("endOffset", a.endOffset); put("label", a.label); put("excerpt", a.excerpt); put("note", a.note)
        put("createdAt", a.createdAt); put("updatedAt", a.updatedAt); put("deleted", a.deleted)
    }

    fun decodeResponse(json: String): AnnotationSyncResponse {
        val root = JSONObject(json)
        fun ids(name: String) = root.optJSONArray(name)?.let { array -> (0 until array.length()).map(array::getString).toSet() } ?: emptySet()
        val items = root.optJSONArray("annotations") ?: JSONArray()
        return AnnotationSyncResponse(
            cursor = root.getLong("cursor"),
            hasMore = root.optBoolean("hasMore"),
            accepted = ids("accepted"),
            rejected = ids("rejected"),
            annotations = (0 until items.length()).map { index ->
                val a = items.getJSONObject(index)
                val l = a.getJSONObject("locator")
                Annotation(
                    bookId = a.getString("bookId"),
                    kind = if (a.getString("kind") == "highlight") AnnotationKind.Highlight else AnnotationKind.Bookmark,
                    locator = Locator(l.getString("kind"), l.optString("value"), l.optInt("page")),
                    label = a.optString("label"), excerpt = a.optString("excerpt"), note = a.optString("note"),
                    createdAt = a.getString("createdAt"), syncId = a.getString("id"), endOffset = a.optInt("endOffset"),
                    updatedAt = a.getString("updatedAt"), deleted = a.optBoolean("deleted"),
                )
            },
        )
    }
}

/**
 * Pushes unsent annotation edits and pulls other devices' changes. Like [SyncEngine], a failed
 * request leaves the queue untouched and throws so the caller retries.
 */
class AnnotationSyncEngine(private val store: AnnotationStore, private val api: ApiClient) {
    /** Returns true when edits are still queued afterwards. */
    fun runOnce(): Boolean {
        repeat(50) {
            val batch = store.pending(100)
            val response = AnnotationCodec.decodeResponse(api.authorized("/api/v1/annotations/sync", "POST", AnnotationCodec.encodeRequest(store.cursor(), batch)))
            store.applySync(batch, response.accepted, response.rejected, response.annotations, response.cursor)
            val settled = response.accepted.size + response.rejected.size
            if (!response.hasMore && (store.pendingCount() == 0 || settled == 0)) return store.pendingCount() > 0
        }
        return store.pendingCount() > 0
    }
}
