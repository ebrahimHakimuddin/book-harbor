package dev.bookharbor.app.library

import org.json.JSONArray
import org.json.JSONObject
import java.net.URLEncoder

data class CatalogFilter(val q: String = "", val format: String = "", val tag: String = "", val series: String = "", val libraryId: String = "") {
    fun toJson(): JSONObject = JSONObject().put("q", q.trim()).put("format", format).put("tag", tag).put("series", series).put("libraryId", libraryId)
    companion object {
        fun fromJson(json: JSONObject) = CatalogFilter(json.optString("q"), json.optString("format"), json.optString("tag"), json.optString("series"), json.optString("libraryId"))
    }
}

data class CatalogLibrary(val id: String, val name: String, val bookCount: Int)
data class SavedCatalogFilter(val id: String, val name: String, val filter: CatalogFilter)

private fun savedFilter(json: JSONObject) = SavedCatalogFilter(json.getString("id"), json.getString("name"), CatalogFilter.fromJson(json.getJSONObject("filter")))
private fun encoded(value: String): String = URLEncoder.encode(value, "UTF-8")

class CatalogClient(private val api: ApiClient) {
    fun search(filter: CatalogFilter, cursor: String? = null): BookPage {
        val fields = listOf("q" to filter.q.trim(), "format" to filter.format, "tag" to filter.tag, "series" to filter.series, "libraryId" to filter.libraryId, "cursor" to cursor.orEmpty())
        val query = fields.filter { it.second.isNotBlank() }.joinToString("") { "&${it.first}=${encoded(it.second)}" }
        return parseBookPage(api.authorized("/api/v1/books?limit=50$query"))
    }

    fun libraries(): List<CatalogLibrary> {
        val items = JSONObject(api.authorized("/api/v1/libraries")).optJSONArray("items") ?: JSONArray()
        return (0 until items.length()).map { items.getJSONObject(it).let { json -> CatalogLibrary(json.getString("id"), json.getString("name"), json.optInt("bookCount")) } }
    }

    fun filters(): List<SavedCatalogFilter> {
        val items = JSONObject(api.authorized("/api/v1/saved-filters")).optJSONArray("items") ?: JSONArray()
        return (0 until items.length()).map { savedFilter(items.getJSONObject(it)) }
    }

    fun saveFilter(name: String, filter: CatalogFilter, id: String? = null): SavedCatalogFilter {
        val path = "/api/v1/saved-filters" + (id?.let { "/${encoded(it)}" } ?: "")
        return savedFilter(JSONObject(api.authorized(path, if (id == null) "POST" else "PUT", JSONObject().put("name", name.trim()).put("filter", filter.toJson()).toString())))
    }

    fun deleteFilter(id: String) { api.authorized("/api/v1/saved-filters/${encoded(id)}", "DELETE") }
}

/** Offline results use the last catalog; permission changes take effect when it next refreshes. */
fun cachedCatalogSearch(books: List<Book>, filter: CatalogFilter): List<Book> {
    val needle = filter.q.trim()
    return books.filter { book ->
        (filter.libraryId.isEmpty() || book.libraryId == filter.libraryId) &&
            (filter.format.isEmpty() || book.editions.any { it.format.equals(filter.format, true) }) &&
            (filter.tag.isEmpty() || book.tags.any { it.equals(filter.tag, true) }) &&
            (filter.series.isEmpty() || book.series.equals(filter.series, true)) &&
            (needle.isEmpty() || (listOf(book.title, book.subtitle, book.description, book.series, book.publisher, book.isbn) + book.authors + book.tags).any { it.contains(needle, true) })
    }
}
