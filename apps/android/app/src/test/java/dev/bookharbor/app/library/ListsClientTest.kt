package dev.bookharbor.app.library
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ListsClientTest {
 @Test fun parsesLists() {
  val json = """{"items":[{"id":"list_1","name":"Beach reads","bookCount":3}]}"""
  val list = parseLists(json).single()
  assertEquals("list_1", list.id); assertEquals("Beach reads", list.name); assertEquals(3, list.bookCount)
 }

 @Test fun parsesEmptyLists() { assertTrue(parseLists("""{"items":[]}""").isEmpty()) }
}
