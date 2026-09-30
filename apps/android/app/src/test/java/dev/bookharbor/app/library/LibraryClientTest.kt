package dev.bookharbor.app.library
import android.content.SharedPreferences
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.URL
import java.nio.file.Files
import java.security.MessageDigest
import kotlinx.coroutines.launch

/** Bare in-memory SharedPreferences: no Robolectric, this JVM test needs no persistence, just the interface. */
private class InMemoryPreferences : SharedPreferences {
 private val values = mutableMapOf<String, Any?>()
 override fun getAll() = values.toMap()
 override fun getString(key: String, defValue: String?) = values[key] as? String ?: defValue
 override fun getStringSet(key: String, defValues: MutableSet<String>?) = @Suppress("UNCHECKED_CAST") (values[key] as? MutableSet<String> ?: defValues)
 override fun getInt(key: String, defValue: Int) = values[key] as? Int ?: defValue
 override fun getLong(key: String, defValue: Long) = values[key] as? Long ?: defValue
 override fun getFloat(key: String, defValue: Float) = values[key] as? Float ?: defValue
 override fun getBoolean(key: String, defValue: Boolean) = values[key] as? Boolean ?: defValue
 override fun contains(key: String) = values.containsKey(key)
 override fun edit(): SharedPreferences.Editor = object : SharedPreferences.Editor {
  private val pending = mutableMapOf<String, Any?>(); private val removed = mutableSetOf<String>(); private var cleared = false
  override fun putString(key: String, value: String?) = apply { pending[key] = value }
  override fun putStringSet(key: String, values: MutableSet<String>?) = apply { pending[key] = values }
  override fun putInt(key: String, value: Int) = apply { pending[key] = value }
  override fun putLong(key: String, value: Long) = apply { pending[key] = value }
  override fun putFloat(key: String, value: Float) = apply { pending[key] = value }
  override fun putBoolean(key: String, value: Boolean) = apply { pending[key] = value }
  override fun remove(key: String) = apply { removed += key }
  override fun clear() = apply { cleared = true }
  override fun commit(): Boolean { apply(); return true }
  override fun apply() { if (cleared) values.clear(); removed.forEach { values.remove(it) }; values.putAll(pending) }
 }
 override fun registerOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) {}
 override fun unregisterOnSharedPreferenceChangeListener(listener: SharedPreferences.OnSharedPreferenceChangeListener?) {}
}

/** An input stream that hands out [bytes] then fails partway through, simulating a dropped connection. */
private class DroppedStream(private val bytes: ByteArray, private val failAfter: Int) : InputStream() {
 private var pos = 0
 override fun read(): Int = throw UnsupportedOperationException()
 override fun read(b: ByteArray, off: Int, len: Int): Int {
  if (pos >= failAfter) throw IOException("connection dropped")
  val n = minOf(len, failAfter - pos, bytes.size - pos)
  System.arraycopy(bytes, pos, b, off, n)
  pos += n
  return n
 }
}

private class FakeConnection(private val body: InputStream, private val code: Int) : HttpURLConnection(URL("http://test")) {
 val headers = mutableMapOf<String, String>()
 override fun connect() {}
 override fun disconnect() {}
 override fun usingProxy() = false
 override fun getResponseCode() = code
 override fun getInputStream(): InputStream = body
 override fun getErrorStream(): InputStream? = null
 override fun setRequestProperty(key: String?, value: String?) { if (key != null && value != null) headers[key] = value }
}

class LibraryClientTest {
 @Test fun browseDiscardsResultsFromAnOlderSearch() = kotlinx.coroutines.runBlocking {
  val session = SessionStore(InMemoryPreferences()).apply { serverUrl = "http://example.test"; tokens = SessionTokens("token", "refresh") }
  val started = kotlinx.coroutines.CompletableDeferred<Unit>()
  val release = java.util.concurrent.CountDownLatch(1)
  val client = CatalogClient(ApiClient(session, openConnection = { url ->
   val old = URL(url).query.contains("q=old")
   if (old) { started.complete(Unit); check(release.await(5, java.util.concurrent.TimeUnit.SECONDS)) }
   val id = if (old) "old" else "new"
   FakeConnection(ByteArrayInputStream("""{"items":[{"id":"$id","title":"$id","editions":[]}],"total":1}""".toByteArray()), 200)
  }))
  val browse = BrowseController(client, this)
  browse.setFilter(CatalogFilter(q = "old"))
  val old = launch { browse.search(emptyList(), false) }
  kotlinx.coroutines.withTimeout(5000) { started.await() }
  browse.setFilter(CatalogFilter(q = "new"))
  release.countDown(); old.join()
  assertEquals(emptyList<Book>(), browse.state.books)
  browse.search(emptyList(), false)
  assertEquals("new", browse.state.books.single().id)
  browse.reset()
  assertEquals(CatalogFilter(), browse.state.filter)
  assertEquals(emptyList<Book>(), browse.state.books)
 }
 @Test fun browsePaginationDeduplicatesAndKeepsResultsOnPageFailure() = kotlinx.coroutines.runBlocking {
  val session = SessionStore(InMemoryPreferences()).apply { serverUrl = "http://example.test"; tokens = SessionTokens("token", "refresh") }
  var fail = false
  val client = CatalogClient(ApiClient(session, openConnection = { url ->
   val more = URL(url).query.contains("cursor=")
   val body = if (more) """{"items":[{"id":"first","editions":[]},{"id":"second","editions":[]}],"total":2}""" else """{"items":[{"id":"first","editions":[]}],"total":2,"nextCursor":"next"}"""
   FakeConnection(ByteArrayInputStream(body.toByteArray()), if (fail) 503 else 200)
  }))
  val browse = BrowseController(client, this)
  browse.search(emptyList(), false)
  fail = true; browse.loadMore()
  kotlinx.coroutines.withTimeout(5000) { while (browse.state.loading) kotlinx.coroutines.delay(10) }
  assertEquals(listOf("first"), browse.state.books.map { it.id })
  assertEquals("next", browse.state.nextCursor)
  assertEquals(true, browse.state.error != null)
  fail = false; browse.loadMore()
  kotlinx.coroutines.withTimeout(5000) { while (browse.state.loading) kotlinx.coroutines.delay(10) }
  assertEquals(listOf("first", "second"), browse.state.books.map { it.id })
  assertEquals(null, browse.state.nextCursor)
  assertEquals(null, browse.state.error)
 }
 @Test fun catalogSearchEncodesEveryFilterAndCursorAndKeepsTotals() {
  val session = SessionStore(InMemoryPreferences()).apply { serverUrl = "http://example.test"; tokens = SessionTokens("token", "refresh") }
  var requested = ""
  val api = ApiClient(session, openConnection = { url ->
   requested = url
   FakeConnection(ByteArrayInputStream("""{"items":[{"id":"book","title":"Book","libraryId":"library_private","editions":[]}],"total":73,"nextCursor":"next+cursor"}""".toByteArray()), 200)
  })
  val page = CatalogClient(api).search(CatalogFilter("  a & b 日本  ", "pdf", "space tag", "A/B", "library_private"), "cursor+&=")
  val fields = URL(requested).query.split('&').associate { it.substringBefore('=') to java.net.URLDecoder.decode(it.substringAfter('='), "UTF-8") }
  assertEquals(mapOf("limit" to "50", "q" to "a & b 日本", "format" to "pdf", "tag" to "space tag", "series" to "A/B", "libraryId" to "library_private", "cursor" to "cursor+&="), fields)
  assertEquals(73, page.total); assertEquals("next+cursor", page.nextCursor)
  assertEquals("library_private", parseBooks(encodeBooks(page.books)).single().libraryId)
 }
 @Test fun savedFilterRoundTripAndRequestsPreserveTheDefinition() {
  val session = SessionStore(InMemoryPreferences()).apply { serverUrl = "http://example.test"; tokens = SessionTokens("token", "refresh") }
  val response = """{"id":"filter_one","name":"Favorites","filter":{"q":"<Book>","format":"epub","tag":"日本","series":"Series","libraryId":"private"}}"""
  val requests = mutableListOf<Pair<String, HttpURLConnection>>()
  val bodies = mutableListOf<java.io.ByteArrayOutputStream>()
  val client = CatalogClient(ApiClient(session, openConnection = { url ->
   val output = java.io.ByteArrayOutputStream(); bodies += output
   object : HttpURLConnection(URL(url)) {
    override fun connect() {}
    override fun disconnect() {}
    override fun usingProxy() = false
    override fun getResponseCode() = if (requestMethod == "DELETE") 204 else 200
    override fun getOutputStream() = output
    override fun getInputStream() = ByteArrayInputStream((if (requestMethod == "GET") "{\"items\":[$response]}" else response).toByteArray())
   }.also { requests += url to it }
  }))
  val saved = client.filters().single()
  assertEquals(CatalogFilter("<Book>", "epub", "日本", "Series", "private"), saved.filter)
  assertEquals(saved, client.saveFilter("Favorites", saved.filter))
  client.saveFilter("Renamed", saved.filter, saved.id)
  client.deleteFilter(saved.id)
  assertEquals(listOf("GET", "POST", "PUT", "DELETE"), requests.map { it.second.requestMethod })
  assertEquals("/api/v1/saved-filters/filter_one", URL(requests[2].first).path)
  assertEquals(saved.filter, CatalogFilter.fromJson(org.json.JSONObject(bodies[2].toString("UTF-8")).getJSONObject("filter")))
  assertEquals("Renamed", org.json.JSONObject(bodies[2].toString("UTF-8")).getString("name"))
 }
 @Test fun offlineSearchUsesAllMetadataAndLibraryFormatTagSeriesTogether() {
  val matching = Book("b", "Book", listOf(Edition("e", "epub", "", "", "")), description = "Hidden phrase", series = "Series", tags = listOf("タグ"), libraryId = "private")
  val others = listOf(matching.copy(id = "other-library", libraryId = "main"), matching.copy(id = "other-format", editions = emptyList()), matching.copy(id = "other-tag", tags = emptyList()), matching.copy(id = "other-series", series = "Other"))
  assertEquals(listOf(matching), cachedCatalogSearch(others + matching, CatalogFilter("hidden phrase", "epub", "タグ", "series", "private")))
 }
 @Test fun richMetadataSurvivesCachingAndSupportsOfflineSearch() {
  val book = parseBooks("""{"items":[{"id":"rich","title":"Book","publisher":"Harbor Press","publishedDate":"2026-09","language":"en-US","isbn":"9780306406157","editions":[]}]}""").single()
  val cached = parseBooks(encodeBooks(listOf(book)))
  assertEquals(listOf(book), cached)
  assertEquals("2026-09", cached.single().publishedDate)
  assertEquals("en-US", cached.single().language)
  assertEquals(cached, cachedCatalogSearch(cached, CatalogFilter("harbor press")))
  assertEquals(cached, cachedCatalogSearch(cached, CatalogFilter("9780306406157")))
  assertEquals(emptyList<Book>(), cachedCatalogSearch(cached, CatalogFilter("other publisher")))
 }
 @Test fun httpErrorsKeepServerGuidanceAndHideUnexpectedBodies() {
  assertEquals("Email is already registered", HttpError(409, """{"code":"email_exists","message":"Email is already registered"}""").message)
  assertEquals("The server is temporarily unavailable. Please try again later.", HttpError(502, "<html>Bad gateway</html>").message)
  assertEquals("Your session has expired. Please sign in again.", HttpError(401, "").message)
  assertEquals("The request couldn't be completed. Please try again.", HttpError(400, """{"detail":"unexpected intermediary response"}""").message)
 }
 @Test fun proxyFailureNeverDisplaysRawResponse() {
  val response = """{"type":"https://developers.cloudflare.com/error-1033","title":"Error 1033: Cloudflare Tunnel error","status":530,"detail":"The host is configured as a Cloudflare Tunnel","cloudflare_error":true}"""
  val api = ApiClient(SessionStore(InMemoryPreferences()), openConnection = {
   object : HttpURLConnection(URL("http://test")) {
    override fun connect() {}
    override fun disconnect() {}
    override fun usingProxy() = false
    override fun getResponseCode() = 530
    override fun getErrorStream() = ByteArrayInputStream(response.toByteArray())
   }
  })
  try { api.request("http://test"); throw AssertionError("expected a connection error") }
  catch (error: HttpError) {
   assertEquals(530, error.status)
   assertEquals("The server is temporarily unreachable. Please try again in a few minutes.", error.message)
  }
 }
 @Test fun parsesModels() { assertEquals("a", SessionTokens.fromJson("{\"accessToken\":\"a\",\"refreshToken\":\"r\"}").accessToken); val books=parseBooks("{\"items\":[{\"id\":\"b\",\"title\":\"Book\",\"editions\":[]}]}"); assertEquals("Book",books.single().title) }
 @Test fun parsesInstance() { assertEquals(false, InstanceInfo.fromJson("{\"name\":\"H\",\"version\":\"1\",\"setupRequired\":false}").setupRequired) }
 @Test fun checksumMatchMovesFile() {
  val dir = Files.createTempDirectory("bookharbor-test").toFile(); val source = File(dir, "part"); val destination = File(dir, "final"); source.writeText("hello")
  val digest = MessageDigest.getInstance("SHA-256").digest("hello".toByteArray()).joinToString("") { "%02x".format(it) }
  EditionDownloader.verifyAndMove(source, destination, 5, digest)
  assertEquals(true, destination.isFile); assertEquals(false, source.exists())
 }
 @Test fun checksumMismatchLeavesNoFinalFile() {
  val dir = Files.createTempDirectory("bookharbor-test").toFile(); val source = File(dir, "part"); val destination = File(dir, "final"); source.writeText("hello")
  try { EditionDownloader.verifyAndMove(source, destination, 5, "00".repeat(32)); throw AssertionError("expected mismatch") } catch (_: DownloadVerificationError) { }
  assertEquals(false, destination.exists())
 }
 @Test fun downloadResumesFromPartialFileInsteadOfRestarting() {
  val dir = Files.createTempDirectory("bookharbor-test").toFile()
  val prefs = InMemoryPreferences()
  val downloads = DownloadStore(prefs, dir)
  val session = SessionStore(prefs)
  session.serverUrl = "http://example.test"; session.tokens = SessionTokens("token", "refresh")
  val full = "hello world resume test".toByteArray()
  val sha = MessageDigest.getInstance("SHA-256").digest(full).joinToString("") { "%02x".format(it) }
  val edition = Edition("e1", "epub", "application/epub+zip", "book.epub", "/content", full.size.toLong(), sha)

  // First attempt: the connection drops after 5 bytes.
  val failing = EditionDownloader(ApiClient(session), downloads, openConnection = { FakeConnection(DroppedStream(full, 5), 200) })
  try { failing.download(edition); throw AssertionError("expected the dropped connection to fail the download") } catch (_: IOException) { }
  val partial = File(dir, "e1.part")
  assertEquals(true, partial.isFile); assertEquals(5L, partial.length())

  // Second attempt: only the remaining bytes are requested and returned (206), not the whole file again.
  val resumed = FakeConnection(ByteArrayInputStream(full.copyOfRange(5, full.size)), 206)
  val resuming = EditionDownloader(ApiClient(session), downloads, openConnection = { resumed })
  val result = resuming.download(edition)
  assertEquals("bytes=5-", resumed.headers["Range"])
  assertEquals(sha, result.sha256)
  assertEquals(String(full), File(result.path).readText())
  assertEquals(false, partial.exists())
 }
}
