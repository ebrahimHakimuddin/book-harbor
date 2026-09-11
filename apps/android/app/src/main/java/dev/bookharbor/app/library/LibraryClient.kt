package dev.bookharbor.app.library

import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedReader
import java.io.File
import java.io.FileInputStream
import java.io.IOException
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.security.MessageDigest
import java.util.Locale

data class InstanceInfo(val name: String, val version: String, val setupRequired: Boolean) {
    companion object { fun fromJson(json: String) = JSONObject(json).let { InstanceInfo(it.optString("name"), it.optString("version"), it.optBoolean("setupRequired")) } }
}

data class Edition(val id: String, val format: String, val mediaType: String, val originalFilename: String, val contentUrl: String, val byteLength: Long = 0, val sha256: String = "")
data class Book(
    val id: String,
    val title: String,
    val editions: List<Edition>,
    val subtitle: String = "",
    val authors: List<String> = emptyList(),
    val coverUrl: String = "",
    val updatedAt: String = "",
)
data class BookPage(val books: List<Book>, val nextCursor: String?)

data class SessionTokens(val accessToken: String, val refreshToken: String, val tokenType: String = "Bearer") {
    companion object { fun fromJson(json: String) = JSONObject(json).let { SessionTokens(it.getString("accessToken"), it.getString("refreshToken"), it.optString("tokenType", "Bearer")) } }
}

fun parseBookPage(json: String): BookPage {
    val root = JSONObject(json)
    val items = root.optJSONArray("items") ?: JSONArray()
    val books = (0 until items.length()).map { index ->
        val book = items.getJSONObject(index)
        val editions = book.optJSONArray("editions") ?: JSONArray()
        val authors = book.optJSONArray("authors") ?: JSONArray()
        Book(
            id = book.getString("id"),
            title = book.optString("title"),
            subtitle = book.optString("subtitle"),
            authors = (0 until authors.length()).map { authors.getString(it) },
            coverUrl = book.optString("coverUrl"),
            updatedAt = book.optString("updatedAt"),
            editions = (0 until editions.length()).map { editionIndex ->
                val edition = editions.getJSONObject(editionIndex)
                Edition(edition.getString("id"), edition.optString("format"), edition.optString("mediaType"), edition.optString("originalFilename"), edition.optString("contentUrl"), edition.optLong("byteLength", 0), edition.optString("sha256"))
            },
        )
    }
    return BookPage(books, root.optString("nextCursor").ifEmpty { null })
}

fun parseBooks(json: String): List<Book> = parseBookPage(json).books

class SessionStore(private val preferences: SharedPreferences) {
    var serverUrl: String get() = preferences.getString("server_url", "") ?: ""; set(value) { preferences.edit().putString("server_url", value.trim().trimEnd('/')).apply() }
    var tokens: SessionTokens? get() = preferences.getString("access_token", null)?.let { SessionTokens(it, preferences.getString("refresh_token", "") ?: "") }; set(value) { preferences.edit().apply { if (value == null) { remove("access_token"); remove("refresh_token") } else { putString("access_token", value.accessToken); putString("refresh_token", value.refreshToken) } }.apply() }
}

data class LocalDownload(val editionId: String, val path: String, val sha256: String)

/** Small, durable index of files owned by this app. The files themselves live in filesDir. */
class DownloadStore(private val preferences: SharedPreferences, private val filesDir: File) {
    internal val directory: File get() = filesDir
    @Synchronized fun get(editionId: String): LocalDownload? {
        val item = read().firstOrNull { it.editionId == editionId && ownedFile(it.path)?.isFile == true } ?: return null
        val file = ownedFile(item.path) ?: return null
        if (item.sha256.isBlank() || sha256(file).equals(item.sha256, ignoreCase = true)) return item
        file.delete(); write(read().filterNot { it.editionId == editionId }); return null
    }
    @Synchronized fun all(): List<LocalDownload> = read().filter { ownedFile(it.path)?.isFile == true }
    @Synchronized fun save(download: LocalDownload) {
        val next = read().filterNot { it.editionId == download.editionId } + download
        write(next)
    }
    @Synchronized fun remove(editionId: String) {
        read().firstOrNull { it.editionId == editionId }?.let { ownedFile(it.path)?.let { file -> if (file.exists() && !file.delete()) throw IOException("Unable to remove local download") } }
        write(read().filterNot { it.editionId == editionId })
    }
    private fun read(): List<LocalDownload> = try {
        val json = preferences.getString(KEY, "[]") ?: "[]"
        val array = JSONArray(json)
        (0 until array.length()).map { val item = array.getJSONObject(it); LocalDownload(item.getString("editionId"), item.getString("path"), item.getString("sha256")) }
    } catch (_: Exception) { emptyList() }
    private fun write(downloads: List<LocalDownload>) {
        val array = JSONArray(); downloads.forEach { array.put(JSONObject().apply { put("editionId", it.editionId); put("path", it.path); put("sha256", it.sha256) }) }
        preferences.edit().putString(KEY, array.toString()).apply()
    }
    private fun sha256(file: File): String { val digest = MessageDigest.getInstance("SHA-256"); file.inputStream().use { input -> val buffer = ByteArray(64 * 1024); while (true) { val count = input.read(buffer); if (count < 0) break; digest.update(buffer, 0, count) } }; return digest.digest().joinToString("") { "%02x".format(it) } }
    private fun ownedFile(path: String): File? = runCatching { val root = filesDir.canonicalFile; val file = File(path).canonicalFile; if (file.path == root.path || file.path.startsWith(root.path + File.separator)) file else null }.getOrNull()
    companion object { private const val KEY = "local_downloads" }
}

class DownloadVerificationError(message: String) : Exception(message)

/** Downloads synchronously; callers must invoke this from Dispatchers.IO. */
class EditionDownloader(
    private val api: ApiClient,
    private val downloads: DownloadStore,
    private val openConnection: (String) -> HttpURLConnection = { URL(it).openConnection() as HttpURLConnection },
) {
    private val session get() = api.session

    fun download(edition: Edition): LocalDownload {
        var token = session.tokens ?: error("sign in required")
        val directory = downloads.directory
        directory.mkdirs()
        val name = safeName(edition.originalFilename.ifBlank { "${edition.id}.${edition.format.ifBlank { "book" }}" })
        val safeId = safeName(edition.id)
        val finalFile = File(directory, "${safeId}-$name")
        val temporary = File.createTempFile("edition-${safeId}-", ".part", directory)
        var activeConnection: HttpURLConnection? = null
        try {
            val contentUrl = URL(URL(session.serverUrl.trimEnd('/') + "/"), edition.contentUrl).toString()
            var status: Int
            var refreshed = false
            while (true) {
                val opened = openConnection(contentUrl)
                activeConnection = opened
                opened.requestMethod = "GET"
                opened.connectTimeout = 15_000
                opened.readTimeout = 30_000
                opened.setRequestProperty("Authorization", "Bearer ${token.accessToken}")
                status = opened.responseCode
                if (status == 401 && !refreshed) { opened.disconnect(); token = api.refreshIfStale(token.accessToken); refreshed = true; continue }
                break
            }
            val connection = activeConnection ?: error("download connection unavailable")
            if (status !in 200..299) throw HttpError(status, connection.errorStream?.let { BufferedReader(InputStreamReader(it)).use { reader -> reader.readText() } } ?: "download failed")
            val digest = MessageDigest.getInstance("SHA-256")
            var downloadedBytes = 0L
            connection.inputStream.use { input -> temporary.outputStream().use { output ->
                val buffer = ByteArray(64 * 1024)
                while (true) { val count = input.read(buffer); if (count < 0) break; output.write(buffer, 0, count); digest.update(buffer, 0, count); downloadedBytes += count }
            } }
            val actualSha = digest.digest().joinToString("") { "%02x".format(it) }
            if (edition.byteLength > 0 && downloadedBytes != edition.byteLength) throw DownloadVerificationError("Downloaded file length does not match the edition")
            if (edition.sha256.isNotBlank() && !actualSha.equals(edition.sha256, ignoreCase = true)) throw DownloadVerificationError("Downloaded file checksum does not match the edition")
            try { Files.move(temporary.toPath(), finalFile.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING) }
            catch (_: Exception) { Files.move(temporary.toPath(), finalFile.toPath(), StandardCopyOption.REPLACE_EXISTING) }
            val result = LocalDownload(edition.id, finalFile.absolutePath, edition.sha256.lowercase(Locale.ROOT))
            downloads.save(result)
            return result
        } catch (error: Exception) {
            temporary.delete()
            throw error
        } finally {
            activeConnection?.disconnect()
        }
    }

    fun remove(editionId: String) = downloads.remove(editionId)

    companion object {
        internal fun verifyAndMove(source: File, destination: File, expectedLength: Long, expectedSha256: String) {
            val digest = MessageDigest.getInstance("SHA-256")
            var length = 0L
            FileInputStream(source).use { input -> val buffer = ByteArray(64 * 1024); while (true) { val count = input.read(buffer); if (count < 0) break; digest.update(buffer, 0, count); length += count } }
            val actual = digest.digest().joinToString("") { "%02x".format(it) }
            if (expectedLength > 0 && length != expectedLength) throw DownloadVerificationError("Downloaded file length does not match the edition")
            if (expectedSha256.isNotBlank() && !actual.equals(expectedSha256, ignoreCase = true)) throw DownloadVerificationError("Downloaded file checksum does not match the edition")
            try { Files.move(source.toPath(), destination.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING) }
            catch (_: Exception) { Files.move(source.toPath(), destination.toPath(), StandardCopyOption.REPLACE_EXISTING) }
        }
        private fun safeName(value: String) = value.replace(Regex("[^A-Za-z0-9._-]"), "_").take(120).ifBlank { "book" }
    }
}


class LibraryClient(private val api: ApiClient) {
    private val store get() = api.session

    fun instance(url: String): InstanceInfo = api.request(url.trimEnd('/') + "/api/v1/instance").let(InstanceInfo::fromJson)

    fun signIn(url: String, email: String, password: String): SessionTokens {
        val body = "{\"email\":${JSONObject.quote(email)},\"password\":${JSONObject.quote(password)}}"
        return SessionTokens.fromJson(api.request(url.trimEnd('/') + "/api/v1/sessions", "POST", body))
            .also { store.serverUrl = url; store.tokens = it }
    }

    fun signOut() {
        val token = store.tokens ?: return
        runCatching { api.request(store.serverUrl + "/api/v1/sessions/current", "DELETE", token = token.accessToken) }
    }

    /** Every book, following the server's pagination cursor. */
    fun books(): List<Book> {
        val all = mutableListOf<Book>()
        var cursor: String? = null
        do {
            val query = "?limit=100" + (cursor?.let { "&cursor=" + java.net.URLEncoder.encode(it, "UTF-8") } ?: "")
            val page = parseBookPage(api.authorized("/api/v1/books$query"))
            all += page.books
            cursor = page.nextCursor
        } while (cursor != null)
        return all
    }
}
