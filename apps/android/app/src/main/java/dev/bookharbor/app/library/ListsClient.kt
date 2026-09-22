package dev.bookharbor.app.library

import org.json.JSONArray
import org.json.JSONObject

/** A reader's list; [bookIds] (newest first) comes with the index, for covers and membership. */
data class BookList(val id: String, val name: String, val bookCount: Int, val bookIds: List<String> = emptyList())

private fun parseList(json: JSONObject) = BookList(
    id = json.getString("id"), name = json.optString("name"), bookCount = json.optInt("bookCount"),
    bookIds = json.optJSONArray("bookIds")?.let { ids -> (0 until ids.length()).map { ids.getString(it) } }.orEmpty(),
)

fun parseLists(json: String): List<BookList> {
    val items = JSONObject(json).optJSONArray("items") ?: JSONArray()
    return (0 until items.length()).map { parseList(items.getJSONObject(it)) }
}

/** A reader's own named lists of books, separate from the fixed shelf filters. */
class ListsClient(private val api: ApiClient) {
    fun lists(): List<BookList> = parseLists(api.authorized("/api/v1/lists"))

    fun create(name: String): BookList = parseList(JSONObject(api.authorized("/api/v1/lists", "POST", "{\"name\":${JSONObject.quote(name)}}")))

    fun rename(id: String, name: String) {
        api.authorized("/api/v1/lists/$id", "PATCH", "{\"name\":${JSONObject.quote(name)}}")
    }

    fun delete(id: String) {
        api.authorized("/api/v1/lists/$id", "DELETE")
    }

    fun books(id: String): List<Book> = parseBooks(api.authorized("/api/v1/lists/$id/books"))

    fun addBook(id: String, bookId: String) {
        api.authorized("/api/v1/lists/$id/books", "POST", "{\"bookId\":${JSONObject.quote(bookId)}}")
    }

    fun removeBook(id: String, bookId: String) {
        api.authorized("/api/v1/lists/$id/books/$bookId", "DELETE")
    }
}
