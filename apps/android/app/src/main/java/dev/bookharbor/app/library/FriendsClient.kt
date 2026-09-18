package dev.bookharbor.app.library

import org.json.JSONArray
import org.json.JSONObject

data class FriendActivityBook(val bookId: String, val title: String, val coverUrl: String, val percentage: Double, val updatedAt: String)
data class Friend(
    val userId: String,
    val displayName: String,
    val email: String,
    val activityVisible: Boolean,
    val currentlyReading: List<FriendActivityBook> = emptyList(),
    val finishedThisYear: Int? = null,
    val goalBooks: Int? = null,
)
data class FriendRequest(val userId: String, val displayName: String, val email: String, val direction: String, val createdAt: String)
data class FriendRequests(val incoming: List<FriendRequest>, val outgoing: List<FriendRequest>)
data class SocialSettings(val activityVisible: Boolean, val goalYear: Int, val goalBooks: Int)

private fun parseActivityBook(json: JSONObject) = FriendActivityBook(
    bookId = json.getString("bookId"), title = json.optString("title"), coverUrl = json.optString("coverUrl"),
    percentage = json.optDouble("percentage", 0.0), updatedAt = json.optString("updatedAt"),
)

private fun parseFriend(json: JSONObject): Friend {
    val reading = json.optJSONArray("currentlyReading") ?: JSONArray()
    return Friend(
        userId = json.getString("userId"), displayName = json.optString("displayName"), email = json.optString("email"),
        activityVisible = json.optBoolean("activityVisible"),
        currentlyReading = (0 until reading.length()).map { parseActivityBook(reading.getJSONObject(it)) },
        finishedThisYear = if (json.isNull("finishedThisYear")) null else json.optInt("finishedThisYear"),
        goalBooks = if (json.isNull("goalBooks")) null else json.optInt("goalBooks"),
    )
}

fun parseFriends(json: String): List<Friend> {
    val items = JSONObject(json).optJSONArray("items") ?: JSONArray()
    return (0 until items.length()).map { parseFriend(items.getJSONObject(it)) }
}

private fun parseFriendRequest(json: JSONObject, direction: String) = FriendRequest(
    userId = json.getString("userId"), displayName = json.optString("displayName"), email = json.optString("email"),
    direction = direction, createdAt = json.optString("createdAt"),
)

fun parseFriendRequests(json: String): FriendRequests {
    val root = JSONObject(json)
    val incoming = root.optJSONArray("incoming") ?: JSONArray()
    val outgoing = root.optJSONArray("outgoing") ?: JSONArray()
    return FriendRequests(
        incoming = (0 until incoming.length()).map { parseFriendRequest(incoming.getJSONObject(it), "incoming") },
        outgoing = (0 until outgoing.length()).map { parseFriendRequest(outgoing.getJSONObject(it), "outgoing") },
    )
}

fun parseSocialSettings(json: String): SocialSettings = JSONObject(json).let {
    SocialSettings(it.optBoolean("activityVisible"), it.optInt("goalYear"), it.optInt("goalBooks"))
}

/** Friend relationships, requests, and the visibility/goal settings that gate them. */
class FriendsClient(private val api: ApiClient) {
    fun friends(): List<Friend> = parseFriends(api.authorized("/api/v1/friends"))

    fun sendRequest(email: String) {
        api.authorized("/api/v1/friends/requests", "POST", "{\"email\":${JSONObject.quote(email)}}")
    }

    fun requests(): FriendRequests = parseFriendRequests(api.authorized("/api/v1/friends/requests"))

    fun accept(userId: String) {
        api.authorized("/api/v1/friends/requests/$userId/accept", "POST")
    }

    /** Declines an incoming request or cancels an outgoing one -- the server resolves which by caller identity. */
    fun removeRequest(userId: String) {
        api.authorized("/api/v1/friends/requests/$userId", "DELETE")
    }

    fun removeFriend(userId: String) {
        api.authorized("/api/v1/friends/$userId", "DELETE")
    }

    fun settings(): SocialSettings = parseSocialSettings(api.authorized("/api/v1/me/social-settings"))

    fun updateSettings(visible: Boolean, goalYear: Int, goalBooks: Int): SocialSettings = parseSocialSettings(
        api.authorized(
            "/api/v1/me/social-settings", "PUT",
            "{\"activityVisible\":$visible,\"goalYear\":$goalYear,\"goalBooks\":$goalBooks}",
        ),
    )
}
