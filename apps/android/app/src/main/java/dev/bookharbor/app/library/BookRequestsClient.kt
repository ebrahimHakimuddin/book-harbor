package dev.bookharbor.app.library

import org.json.JSONArray
import org.json.JSONObject

/** A Hardcover (or whichever provider is configured) search hit, used to file a book request. */
data class MetadataCandidate(val provider: String, val id: String, val title: String, val authors: List<String>, val coverUrl: String)

data class BookRequest(
    val id: String,
    val title: String,
    val author: String,
    val coverUrl: String,
    val status: String,
    val createdAt: String,
)

private fun parseCandidate(json: JSONObject): MetadataCandidate {
    val authors = json.optJSONArray("authors") ?: JSONArray()
    return MetadataCandidate(
        provider = json.optString("provider"), id = json.optString("id"), title = json.optString("title"),
        authors = (0 until authors.length()).map { authors.getString(it) }, coverUrl = json.optString("coverUrl"),
    )
}

fun parseCandidates(json: String): List<MetadataCandidate> {
    val items = JSONObject(json).optJSONArray("items") ?: JSONArray()
    return (0 until items.length()).map { parseCandidate(items.getJSONObject(it)) }
}

private fun parseBookRequest(json: JSONObject) = BookRequest(
    id = json.getString("id"), title = json.optString("title"), author = json.optString("author"),
    coverUrl = json.optString("coverUrl"), status = json.optString("status"), createdAt = json.optString("createdAt"),
)

fun parseBookRequests(json: String): List<BookRequest> {
    val items = JSONObject(json).optJSONArray("items") ?: JSONArray()
    return (0 until items.length()).map { parseBookRequest(items.getJSONObject(it)) }
}

/** Searching for a book to request, filing the request, and tracking its status. */
class BookRequestsClient(private val api: ApiClient) {
    fun search(query: String): List<MetadataCandidate> =
        parseCandidates(api.authorized("/api/v1/metadata/search?q=" + java.net.URLEncoder.encode(query, "UTF-8")))

    fun request(candidate: MetadataCandidate) {
        val body = JSONObject().apply {
            put("title", candidate.title)
            put("author", candidate.authors.joinToString(", "))
            put("coverUrl", candidate.coverUrl)
            put("sourceProvider", candidate.provider)
            put("sourceId", candidate.id)
        }
        api.authorized("/api/v1/book-requests", "POST", body.toString())
    }

    fun mine(): List<BookRequest> = parseBookRequests(api.authorized("/api/v1/book-requests"))

    fun cancel(id: String) {
        api.authorized("/api/v1/book-requests/$id", "DELETE")
    }
}
