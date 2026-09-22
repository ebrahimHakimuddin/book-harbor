package dev.bookharbor.app.sync

import java.time.Instant
import java.util.UUID

/**
 * Turns "the reader is here" into a durable event. Readers call this from any thread;
 * the write completes (position + outbox) before it returns, so the UI can trust it.
 */
class ProgressRecorder(
    private val store: ProgressStore,
    private val deviceId: String,
    private val clock: () -> Instant = Instant::now,
    private val onRecorded: () -> Unit = {},
) {
    /** Returns the event, or null when nothing changed since the last recorded position. */
    fun record(bookId: String, editionId: String, locator: Locator, percentage: Double): ProgressEvent? {
        val clamped = percentage.coerceIn(0.0, 1.0)
        // A changed percentage at the same place still counts: marking a book read or unread.
        store.position(bookId)?.let { if (it.editionId == editionId && it.locator == locator && it.percentage == clamped) return null }
        val event = ProgressEvent(
            eventId = "progress_" + UUID.randomUUID().toString().replace("-", ""),
            deviceId = deviceId, bookId = bookId, editionId = editionId,
            occurredAt = clock().toString(), locator = locator, percentage = clamped,
        )
        store.record(event)
        onRecorded()
        return event
    }
}
