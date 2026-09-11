package dev.bookharbor.app.sync

/** One request to the server. Implementations throw [PermanentSyncError] for an unfixable 4xx. */
fun interface SyncApi {
    fun sync(cursor: Long, changes: List<ProgressEvent>): SyncResponse
}

data class SyncOutcome(val sent: Int, val rejected: Int, val pages: Int)

/**
 * Sends the outbox and pulls canonical positions. It never deletes queued work on failure:
 * transient errors propagate so the caller (WorkManager) retries the same event IDs.
 */
class SyncEngine(
    private val store: ProgressStore,
    private val api: SyncApi,
    private val batchSize: Int = 100,
    private val maxPages: Int = 50,
) {
    fun runOnce(): SyncOutcome {
        var sent = 0
        var rejected = 0
        var pages = 0
        while (pages < maxPages) {
            pages++
            val batch = store.pending(batchSize)
            val response = try {
                api.sync(store.cursor(), batch)
            } catch (error: PermanentSyncError) {
                // One bad event must not wedge the queue: find it by sending events singly.
                if (batch.size > 1) {
                    val (ok, bad) = isolate(batch)
                    sent += ok
                    rejected += bad
                } else {
                    batch.forEach { store.reject(it.eventId, error.message ?: "rejected") }
                    rejected += batch.size
                }
                continue
            }
            store.applySync(response.acknowledgements.map { it.eventId }.toSet(), response.progress, response.cursor)
            sent += response.acknowledgements.size
            // Stop when nothing is left to send or pull; also stop if the server acknowledged
            // nothing, so an unexpected response cannot spin this loop.
            val more = response.hasMore || store.pending(1).isNotEmpty()
            if (!more || (!response.hasMore && response.acknowledgements.isEmpty())) break
        }
        return SyncOutcome(sent, rejected, pages)
    }

    private fun isolate(batch: List<ProgressEvent>): Pair<Int, Int> {
        var sent = 0
        var rejected = 0
        for (event in batch) {
            try {
                val response = api.sync(store.cursor(), listOf(event))
                store.applySync(response.acknowledgements.map { it.eventId }.toSet(), response.progress, response.cursor)
                sent += response.acknowledgements.size
            } catch (error: PermanentSyncError) {
                store.reject(event.eventId, error.message ?: "rejected")
                rejected++
            }
        }
        return sent to rejected
    }
}
