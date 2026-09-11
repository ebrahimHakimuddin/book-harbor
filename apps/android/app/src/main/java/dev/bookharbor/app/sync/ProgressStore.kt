package dev.bookharbor.app.sync

/**
 * Durable local reading state: the current position per book, an outbox of events not
 * yet acknowledged by the server, and the server cursor. Every method is one transaction.
 */
interface ProgressStore {
    /** Updates the local position and queues [event] in a single transaction. */
    fun record(event: ProgressEvent)

    fun position(bookId: String): LocalPosition?
    fun positions(): List<LocalPosition>

    /** Oldest events still waiting to be sent (rejected events are excluded). */
    fun pending(limit: Int): List<ProgressEvent>
    fun pendingCount(): Int
    fun rejectedCount(): Int

    fun cursor(): Long

    /**
     * After a successful exchange, atomically: remove only [acknowledged] events, merge
     * [server] positions that are newer than local ones and not shadowed by an unsent
     * local event, and store [cursor].
     */
    fun applySync(acknowledged: Set<String>, server: List<ServerProgress>, cursor: Long)

    /** Parks an event the server will never accept, so it stops blocking the queue. */
    fun reject(eventId: String, reason: String)

    /** Forgets all local progress, the outbox, and the cursor (used when signing out). */
    fun clear()
}
