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
