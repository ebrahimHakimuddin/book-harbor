package dev.bookharbor.app.library
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File
import java.nio.file.Files
import java.security.MessageDigest
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
}
