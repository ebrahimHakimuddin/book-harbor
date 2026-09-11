package dev.bookharbor.app.sync

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException
import java.time.Instant

/** A [ProgressStore] with the same semantics as the SQLite one, for JVM tests. */
class MemoryProgressStore : ProgressStore {
    private val positions = linkedMapOf<String, LocalPosition>()
    private val outbox = linkedMapOf<String, Pair<ProgressEvent, String>>() // eventId -> event, status
    private var cursor = 0L

    override fun record(event: ProgressEvent) {
        positions[event.bookId] = LocalPosition(event.bookId, event.editionId, event.locator, event.percentage, event.occurredAt, event.eventId)
        outbox.putIfAbsent(event.eventId, event to "pending")
    }
    override fun position(bookId: String) = positions[bookId]
    override fun positions() = positions.values.toList()
    override fun pending(limit: Int) = outbox.values.filter { it.second == "pending" }.map { it.first }.take(limit)
    override fun pendingCount() = outbox.values.count { it.second == "pending" }
    override fun rejectedCount() = outbox.values.count { it.second == "rejected" }
    override fun cursor() = cursor
    override fun applySync(acknowledged: Set<String>, server: List<ServerProgress>, cursor: Long) {
        acknowledged.forEach { outbox.remove(it) }
        for (incoming in server) {
            val unsent = outbox.values.any { it.second == "pending" && it.first.bookId == incoming.bookId }
            if (shouldAdopt(positions[incoming.bookId], incoming, unsent)) {
                positions[incoming.bookId] = LocalPosition(incoming.bookId, incoming.editionId, incoming.locator, incoming.percentage, incoming.occurredAt, incoming.eventId)
            }
        }
        this.cursor = cursor
    }
    override fun reject(eventId: String, reason: String) { outbox[eventId]?.let { outbox[eventId] = it.first to "rejected" } }
}

class SyncEngineTest {
    private val t0 = Instant.parse("2026-09-21T10:00:00Z")
    private fun event(id: String, book: String = "book1", page: Int = 1, at: Instant = t0) =
        ProgressEvent(id, "device_a", book, "ed_$book", at.toString(), Locator.pdf(page), 0.1)
    private fun ack(vararg ids: String) = ids.map { Acknowledgement(it, 1, "applied", false) }
    private fun response(acks: List<Acknowledgement> = emptyList(), progress: List<ServerProgress> = emptyList(), cursor: Long = 1, more: Boolean = false) =
        SyncResponse(cursor, more, acks, progress)
    private fun server(book: String, at: Instant, page: Int) = ServerProgress(9, "srv_event", "device_b", book, "ed_$book", at.toString(), Locator.pdf(page), 0.9)

    @Test fun failedSyncKeepsEveryQueuedEvent() {
        val store = MemoryProgressStore().apply { record(event("e1")); record(event("e2", book = "book2")) }
        val engine = SyncEngine(store, { _, _ -> throw IOException("offline") })
        assertThrows(IOException::class.java) { engine.runOnce() }
        assertEquals(listOf("e1", "e2"), store.pending(10).map { it.eventId })
        assertEquals(0L, store.cursor())
    }

    @Test fun removesOnlyAcknowledgedEventsAndStoresTheCursor() {
        val store = MemoryProgressStore().apply { record(event("e1")); record(event("e2", book = "b2")); record(event("e3", book = "b3")) }
        val engine = SyncEngine(store, { _, _ -> response(ack("e1", "e3"), cursor = 42) })
        engine.runOnce()
        assertEquals(listOf("e2"), store.pending(10).map { it.eventId })
        assertEquals(42L, store.cursor())
    }

    @Test fun sendsAtMostOneBatchAtATimeAndFollowsPages() {
        val store = MemoryProgressStore().apply { (1..5).forEach { record(event("e$it", book = "b$it")) } }
        val sizes = mutableListOf<Int>()
        val engine = SyncEngine(store, { _, changes -> sizes += changes.size; response(ack(*changes.map { it.eventId }.toTypedArray()), cursor = sizes.size.toLong()) }, batchSize = 2)
        val outcome = engine.runOnce()
        assertEquals(5, outcome.sent)
        assertEquals(listOf(2, 2, 1), sizes)
        assertEquals(0, store.pendingCount())
    }

    @Test fun adoptsANewerServerPositionWhenNothingIsUnsent() {
        val store = MemoryProgressStore().apply { record(event("e1", page = 3, at = t0)) }
        val engine = SyncEngine(store, { _, _ -> response(ack("e1"), listOf(server("book1", t0.plusSeconds(60), 40))) })
        engine.runOnce()
        assertEquals(40, store.position("book1")!!.locator.page)
    }

    @Test fun keepsFreshOfflineProgressUntilTheServerHasSeenIt() {
        val store = MemoryProgressStore().apply { record(event("e1", page = 3, at = t0)) }
        // The server returns a newer position from another device but does not acknowledge e1 yet.
        val engine = SyncEngine(store, { _, _ -> response(emptyList(), listOf(server("book1", t0.plusSeconds(60), 40))) })
        engine.runOnce()
        assertEquals(3, store.position("book1")!!.locator.page)
        assertEquals(1, store.pendingCount())
    }

    @Test fun neverMovesBackwardsToAnOlderServerPosition() {
        val store = MemoryProgressStore().apply { record(event("e1", page = 50, at = t0)) }
        val engine = SyncEngine(store, { _, _ -> response(ack("e1"), listOf(server("book1", t0.minusSeconds(600), 7))) })
        engine.runOnce()
        assertEquals(50, store.position("book1")!!.locator.page)
    }

    @Test fun oneUnacceptableEventDoesNotBlockTheRest() {
        val store = MemoryProgressStore().apply { record(event("good1", book = "b1")); record(event("bad", book = "b2")); record(event("good2", book = "b3")) }
        val engine = SyncEngine(store, { _, changes ->
            if (changes.any { it.eventId == "bad" }) throw PermanentSyncError(422, "invalid locator")
            response(ack(*changes.map { it.eventId }.toTypedArray()))
        })
        val outcome = engine.runOnce()
        assertEquals(1, outcome.rejected)
        assertEquals(0, store.pendingCount())
        assertEquals(1, store.rejectedCount())
    }

    @Test fun recorderWritesPositionAndEventTogetherAndSkipsRepeats() {
        val store = MemoryProgressStore()
        var recorded = 0
        val recorder = ProgressRecorder(store, "device_a", { t0 }, { recorded++ })
        val first = recorder.record("b1", "ed1", Locator.pdf(5), 1.7)
        assertNotNull(first)
        assertEquals(1.0, first!!.percentage, 0.0) // clamped
        assertEquals(5, store.position("b1")!!.locator.page)
        assertEquals(1, store.pendingCount())
        assertNull(recorder.record("b1", "ed1", Locator.pdf(5), 0.5)) // unchanged locator
        assertNotNull(recorder.record("b1", "ed1", Locator.pdf(6), 0.6))
        assertEquals(2, recorded)
    }

    @Test fun codecMatchesTheServerContract() {
        val json = JSONObject(ProgressCodec.encodeRequest(7, listOf(event("e1"), event("e2").copy(locator = Locator.epub("epubcfi(/6/4!/4/2)")))))
        assertEquals(7, json.getInt("cursor"))
        val changes = json.getJSONArray("changes")
        assertEquals("pdf-page", changes.getJSONObject(0).getJSONObject("locator").getString("kind"))
        assertEquals(1, changes.getJSONObject(0).getJSONObject("locator").getInt("page"))
        assertFalse(changes.getJSONObject(0).getJSONObject("locator").has("value"))
        assertEquals("epubcfi(/6/4!/4/2)", changes.getJSONObject(1).getJSONObject("locator").getString("value"))

        val decoded = ProgressCodec.decodeResponse(
            """{"cursor":12,"hasMore":true,"acknowledgements":[{"eventId":"e1","revision":3,"disposition":"applied","duplicate":true}],
               "progress":[{"revision":3,"eventId":"e1","deviceId":"d","bookId":"b","editionId":"ed","occurredAt":"2026-09-21T10:00:00Z","locator":{"kind":"pdf-page","page":9},"percentage":0.4}]}""",
        )
        assertTrue(decoded.hasMore)
        assertEquals(12L, decoded.cursor)
        assertTrue(decoded.acknowledgements.single().duplicate)
        assertEquals(9, decoded.progress.single().locator.page)
    }
}
