package dev.bookharbor.app.library
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class FriendsClientTest {
 @Test fun parsesFriendWithVisibleActivity() {
  val json = """{"items":[{"userId":"usr_bob","displayName":"Bob","email":"bob@example.com","activityVisible":true,
   "currentlyReading":[{"bookId":"b1","title":"Dune","coverUrl":"/covers/b1","percentage":0.42,"updatedAt":"2026-09-20T00:00:00Z"}],
   "finishedThisYear":7,"goalBooks":20}]}"""
  val friend = parseFriends(json).single()
  assertEquals("usr_bob", friend.userId); assertEquals("Bob", friend.displayName); assertTrue(friend.activityVisible)
  assertEquals(1, friend.currentlyReading.size); assertEquals("Dune", friend.currentlyReading[0].title); assertEquals(0.42, friend.currentlyReading[0].percentage, 0.0001)
  assertEquals(7, friend.finishedThisYear); assertEquals(20, friend.goalBooks)
 }

 @Test fun parsesFriendWithPrivateActivityAsNullFields() {
  val json = """{"items":[{"userId":"usr_bob","displayName":"Bob","email":"bob@example.com","activityVisible":false,
   "currentlyReading":[],"finishedThisYear":null,"goalBooks":null}]}"""
  val friend = parseFriends(json).single()
  assertTrue(friend.currentlyReading.isEmpty()); assertNull(friend.finishedThisYear); assertNull(friend.goalBooks)
 }

 @Test fun parsesEmptyFriendsList() { assertTrue(parseFriends("""{"items":[]}""").isEmpty()) }

 @Test fun parsesIncomingAndOutgoingRequests() {
  val json = """{"incoming":[{"userId":"usr_a","displayName":"Alice","email":"alice@example.com","createdAt":"2026-09-20T00:00:00Z"}],
   "outgoing":[{"userId":"usr_c","displayName":"Cara","email":"cara@example.com","createdAt":"2026-09-21T00:00:00Z"}]}"""
  val requests = parseFriendRequests(json)
  assertEquals(1, requests.incoming.size); assertEquals("incoming", requests.incoming[0].direction); assertEquals("usr_a", requests.incoming[0].userId)
  assertEquals(1, requests.outgoing.size); assertEquals("outgoing", requests.outgoing[0].direction); assertEquals("usr_c", requests.outgoing[0].userId)
 }

 @Test fun parsesSocialSettings() {
  val settings = parseSocialSettings("""{"activityVisible":true,"goalYear":2026,"goalBooks":24}""")
  assertTrue(settings.activityVisible); assertEquals(2026, settings.goalYear); assertEquals(24, settings.goalBooks)
 }

 @Test fun parsesSocialSettingsDefaults() {
  val settings = parseSocialSettings("""{"activityVisible":false,"goalYear":0,"goalBooks":0}""")
  assertEquals(0, settings.goalYear); assertEquals(0, settings.goalBooks)
 }
}
