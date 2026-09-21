package dev.bookharbor.app.library
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class BookRequestsClientTest {
 @Test fun parsesCandidates() {
  val json = """{"provider":"hardcover","items":[{"provider":"hardcover","id":"hc_1","title":"Dune","authors":["Frank Herbert"],"coverUrl":"/c/1"}]}"""
  val candidate = parseCandidates(json).single()
  assertEquals("hardcover", candidate.provider); assertEquals("hc_1", candidate.id); assertEquals("Dune", candidate.title)
  assertEquals(listOf("Frank Herbert"), candidate.authors); assertEquals("/c/1", candidate.coverUrl)
 }

 @Test fun parsesEmptyCandidates() { assertTrue(parseCandidates("""{"items":[]}""").isEmpty()) }

 @Test fun parsesBookRequests() {
  val json = """{"items":[{"id":"req_1","title":"Dune","author":"Frank Herbert","coverUrl":"/c/1","status":"open","createdAt":"2026-09-20T00:00:00Z"}]}"""
  val request = parseBookRequests(json).single()
  assertEquals("req_1", request.id); assertEquals("Dune", request.title); assertEquals("open", request.status)
 }

 @Test fun parsesEmptyBookRequests() { assertTrue(parseBookRequests("""{"items":[]}""").isEmpty()) }
}
