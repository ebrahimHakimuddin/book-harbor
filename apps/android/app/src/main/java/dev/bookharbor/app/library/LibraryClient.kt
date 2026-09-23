package dev.bookharbor.app.library

import android.content.SharedPreferences
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedReader
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.io.IOException
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.security.MessageDigest
import java.util.Locale

data class InstanceInfo(val name: String, val version: String, val setupRequired: Boolean, val passwordResetEnabled: Boolean = false) {
    companion object { fun fromJson(json: String) = JSONObject(json).let { InstanceInfo(it.optString("name"), it.optString("version"), it.optBoolean("setupRequired"), it.optBoolean("passwordResetEnabled")) } }
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
    val description: String = "",
    val series: String = "",
    /** Position within [series]; 0 when unnumbered. */
    val seriesIndex: Double = 0.0,
    val tags: List<String> = emptyList(),
    /** Chapters in the book so far when it's a followed web novel; 0 for any other book. */
    val webnovelChapters: Int = 0,
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
        val tags = book.optJSONArray("tags") ?: JSONArray()
        Book(
            id = book.getString("id"),
            title = book.optString("title"),
            subtitle = book.optString("subtitle"),
            authors = (0 until authors.length()).map { authors.getString(it) },
            coverUrl = book.optString("coverUrl"),
            updatedAt = book.optString("updatedAt"),
            description = book.optString("description"),
            series = book.optString("series"),
            seriesIndex = book.optDouble("seriesIndex", 0.0).takeUnless { it.isNaN() } ?: 0.0,
            tags = (0 until tags.length()).map { tags.getString(it) },
            webnovelChapters = book.optInt("webnovelChapters", 0),
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
    var displayName: String get() = preferences.getString("display_name", "") ?: ""; set(value) { preferences.edit().putString("display_name", value).apply() }
    var tokens: SessionTokens? get() = preferences.getString("access_token", null)?.let { SessionTokens(it, preferences.getString("refresh_token", "") ?: "") }; set(value) { preferences.edit().apply { if (value == null) { remove("access_token"); remove("refresh_token") } else { putString("access_token", value.accessToken); putString("refresh_token", value.refreshToken) } }.apply() }
}

/**
 * A file this app downloaded. [verifiedSize]/[verifiedModified] record the file as it was when its
 * checksum last matched, so opening a book doesn't re-hash the whole file every time.
 */
data class LocalDownload(val editionId: String, val path: String, val sha256: String, val verifiedSize: Long = 0, val verifiedModified: Long = 0)

/** Small, durable index of files owned by this app. The files themselves live in filesDir. */
class DownloadStore(private val preferences: SharedPreferences, private val filesDir: File) {
    internal val directory: File get() = filesDir
    /**
     * The download, verified: a full re-hash only when the file's size or modified time differs
     * from when it last checked out (it was changed or damaged); a mismatch deletes it.
     */
    @Synchronized fun get(editionId: String): LocalDownload? {
        val item = read().firstOrNull { it.editionId == editionId && ownedFile(it.path)?.isFile == true } ?: return null
        val file = ownedFile(item.path) ?: return null
        if (item.verifiedSize > 0 && item.verifiedSize == file.length() && item.verifiedModified == file.lastModified()) return item
        if (item.sha256.isBlank() || sha256(file).equals(item.sha256, ignoreCase = true)) {
            return stamped(item, file).also { save(it) }
        }
        file.delete(); write(read().filterNot { it.editionId == editionId }); return null
    }

    /** [download] marked as verified against [file] as it is now. */
    fun stamped(download: LocalDownload, file: File) = download.copy(verifiedSize = file.length(), verifiedModified = file.lastModified())
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
        (0 until array.length()).map { val item = array.getJSONObject(it); LocalDownload(item.getString("editionId"), item.getString("path"), item.getString("sha256"), item.optLong("verifiedSize"), item.optLong("verifiedModified")) }
    } catch (_: Exception) { emptyList() }
    private fun write(downloads: List<LocalDownload>) {
        val array = JSONArray(); downloads.forEach { array.put(JSONObject().apply { put("editionId", it.editionId); put("path", it.path); put("sha256", it.sha256); put("verifiedSize", it.verifiedSize); put("verifiedModified", it.verifiedModified) }) }
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

    /** [onProgress] gets 0..1 as bytes arrive, when the edition's size is known. */
    fun download(edition: Edition, onProgress: (Float) -> Unit = {}): LocalDownload {
        var token = session.tokens ?: error("sign in required")
        val directory = downloads.directory
        directory.mkdirs()
        val name = safeName(edition.originalFilename.ifBlank { "${edition.id}.${edition.format.ifBlank { "book" }}" })
        val safeId = safeName(edition.id)
        val finalFile = File(directory, "${safeId}-$name")
        // A stable name (not File.createTempFile's random suffix) so a retry after a killed
        // process or a dropped connection finds its own partial data instead of starting over.
        val temporary = File(directory, "$safeId.part")
        var activeConnection: HttpURLConnection? = null
        try {
            val contentUrl = URL(URL(session.serverUrl.trimEnd('/') + "/"), edition.contentUrl).toString()
            val digest = MessageDigest.getInstance("SHA-256")
            var downloadedBytes = if (temporary.isFile) seedDigest(digest, temporary) else 0L
            var status: Int
            var refreshed = false
            while (true) {
                val opened = openConnection(contentUrl)
                activeConnection = opened
                opened.requestMethod = "GET"
                opened.connectTimeout = 15_000
                opened.readTimeout = 30_000
                opened.setRequestProperty("Authorization", "Bearer ${token.accessToken}")
                if (downloadedBytes > 0) opened.setRequestProperty("Range", "bytes=$downloadedBytes-")
                status = opened.responseCode
                if (status == 401 && !refreshed) { opened.disconnect(); token = api.refreshIfStale(token.accessToken); refreshed = true; continue }
                break
            }
            val connection = activeConnection ?: error("download connection unavailable")
            // Asked to resume but the server sent the whole thing again (no range support):
            // the partial bytes on disk are actually the start of a fresh copy, not a
            // continuation, so drop them rather than corrupt the file by appending past them.
            if (downloadedBytes > 0 && status == 200) {
                digest.reset()
                downloadedBytes = 0L
            }
            if (status !in 200..299) throw HttpError(status, connection.errorStream?.let { BufferedReader(InputStreamReader(it)).use { reader -> reader.readText() } } ?: "download failed")
            val append = status == 206
            connection.inputStream.use { input -> FileOutputStream(temporary, append).use { output ->
                val buffer = ByteArray(64 * 1024)
                var reported = -1
                while (true) {
                    val count = input.read(buffer); if (count < 0) break
                    output.write(buffer, 0, count); digest.update(buffer, 0, count); downloadedBytes += count
                    // Whole percents only, so a fast download doesn't flood the UI with updates.
                    if (edition.byteLength > 0) { val percent = (downloadedBytes * 100 / edition.byteLength).toInt(); if (percent != reported) { reported = percent; onProgress(percent / 100f) } }
                }
            } }
            val actualSha = digest.digest().joinToString("") { "%02x".format(it) }
            if (edition.byteLength > 0 && downloadedBytes != edition.byteLength) throw DownloadVerificationError("Downloaded file length does not match the edition")
            if (edition.sha256.isNotBlank() && !actualSha.equals(edition.sha256, ignoreCase = true)) throw DownloadVerificationError("Downloaded file checksum does not match the edition")
            try { Files.move(temporary.toPath(), finalFile.toPath(), StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING) }
            catch (_: Exception) { Files.move(temporary.toPath(), finalFile.toPath(), StandardCopyOption.REPLACE_EXISTING) }
            val result = downloads.stamped(LocalDownload(edition.id, finalFile.absolutePath, edition.sha256.lowercase(Locale.ROOT)), finalFile)
            downloads.save(result)
            return result
        } catch (error: Exception) {
            // A verified-bad file can't be resumed from; anything else (network drop, timeout)
            // keeps the partial bytes on disk so the next attempt resumes instead of restarting.
            if (error is DownloadVerificationError) temporary.delete()
            throw error
        } finally {
            activeConnection?.disconnect()
        }
    }

    fun remove(editionId: String) = downloads.remove(editionId)

    /** Hashes bytes already on disk from a prior attempt so a resumed download's checksum still covers them. */
    private fun seedDigest(digest: MessageDigest, partial: File): Long {
        var length = 0L
        FileInputStream(partial).use { input -> val buffer = ByteArray(64 * 1024); while (true) { val count = input.read(buffer); if (count < 0) break; digest.update(buffer, 0, count); length += count } }
        return length
    }

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
        val json = api.request(url.trimEnd('/') + "/api/v1/sessions", "POST", body)
        return SessionTokens.fromJson(json).also {
            store.serverUrl = url
            store.tokens = it
            store.displayName = runCatching { JSONObject(json).getJSONObject("user").getString("displayName") }.getOrDefault("")
        }
    }

    fun signOut() {
        val token = store.tokens ?: return
        runCatching { api.request(store.serverUrl + "/api/v1/sessions/current", "DELETE", token = token.accessToken) }
    }

    /** Changes the signed-in user's own display name and/or password (the server requires
     * [currentPassword] whenever [newPassword] is set). Returns the (possibly unchanged)
     * display name, and updates the cached copy used elsewhere in the app. */
    fun updateSelf(displayName: String? = null, currentPassword: String? = null, newPassword: String? = null): String {
        val body = JSONObject().apply {
            displayName?.let { put("displayName", it) }
            currentPassword?.let { put("currentPassword", it) }
            newPassword?.let { put("newPassword", it) }
        }.toString()
        val json = api.authorized("/api/v1/me", "PATCH", body)
        val updated = JSONObject(json).getString("displayName")
        store.displayName = updated
        return updated
    }

    /** Emails a reset code if [email] has an account; the server answers the same either way. */
    fun requestPasswordReset(url: String, email: String) {
        api.request(url.trimEnd('/') + "/api/v1/password-resets", "POST", JSONObject().put("email", email).toString())
    }

    fun confirmPasswordReset(url: String, email: String, code: String, newPassword: String) {
        api.request(url.trimEnd('/') + "/api/v1/password-resets/confirm", "POST", JSONObject().put("email", email).put("code", code).put("newPassword", newPassword).toString())
    }

    /** One book as the server has it now. */
    fun book(id: String): Book = parseBookPage("{\"items\":[" + api.authorized("/api/v1/books/" + java.net.URLEncoder.encode(id, "UTF-8")) + "]}").books.first()

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

/** The catalog as last fetched, so the library still opens with no network. */
class CatalogCache(private val preferences: SharedPreferences) {
    fun save(instanceName: String, books: List<Book>) {
        preferences.edit().putString("catalog_books", encodeBooks(books)).putString("catalog_instance", instanceName).apply()
    }

    fun load(): Pair<String, List<Book>>? {
        val json = preferences.getString("catalog_books", null) ?: return null
        return runCatching { (preferences.getString("catalog_instance", "BookHarbor") ?: "BookHarbor") to parseBooks(json) }.getOrNull()
    }

    fun clear() { preferences.edit().remove("catalog_books").remove("catalog_instance").apply() }
}

/** Same shape the server sends, so parseBooks reads it back. */
fun encodeBooks(books: List<Book>): String = JSONObject().put("items", JSONArray().also { items ->
    books.forEach { book ->
        items.put(JSONObject().apply {
            put("id", book.id); put("title", book.title); put("subtitle", book.subtitle); put("coverUrl", book.coverUrl); put("updatedAt", book.updatedAt)
            put("authors", JSONArray(book.authors))
            put("description", book.description); put("series", book.series); put("seriesIndex", book.seriesIndex)
            put("tags", JSONArray(book.tags)); put("webnovelChapters", book.webnovelChapters)
            put("editions", JSONArray().also { editions ->
                book.editions.forEach { e ->
                    editions.put(JSONObject().put("id", e.id).put("format", e.format).put("mediaType", e.mediaType).put("originalFilename", e.originalFilename).put("contentUrl", e.contentUrl).put("byteLength", e.byteLength).put("sha256", e.sha256))
                }
            })
        })
    }
}).toString()
