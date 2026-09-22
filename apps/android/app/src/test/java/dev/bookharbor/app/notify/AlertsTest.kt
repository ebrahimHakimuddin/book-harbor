package dev.bookharbor.app.notify

import org.junit.Assert.assertEquals
import org.junit.Test

class AlertsTest {
    private val before = Snapshot(mapOf("b1" to "Dune"), mapOf("r1" to "Old"), emptyMap())

    @Test fun nothingIsNewOnAFreshSignIn() {
        assertEquals(emptyList<Alert>(), alertsFor(null, before))
    }

    @Test fun announcesOnlyWhatAppearedSinceLastCheck() {
        val now = Snapshot(mapOf("b1" to "Dune", "b2" to "Emma", "b3" to "Kim"), mapOf("r1" to "Old", "r2" to "Emma"), mapOf("u1" to "Sam"))
        val alerts = alertsFor(before, now)
        assertEquals(listOf(1, 2, 3), alerts.map { it.id })
        assertEquals("2 new books in your library", alerts[0].title)
        assertEquals("Emma, Kim", alerts[0].text)
        assertEquals("Emma is now in the library", alerts[1].text)
        assertEquals("Sam wants to be friends", alerts[2].text)
        assertEquals(emptyList<Alert>(), alertsFor(now, now))
    }
}
