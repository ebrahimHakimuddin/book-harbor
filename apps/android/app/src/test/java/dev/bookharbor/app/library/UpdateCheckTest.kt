package dev.bookharbor.app.library

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class UpdateCheckTest {
    @Test fun comparesNumerically() {
        assertTrue(isNewer("0.10.0", "0.9.1"))
        assertTrue(isNewer("0.2.1", "0.2"))
        assertFalse(isNewer("0.2.1", "0.2.1"))
        assertFalse(isNewer("0.2.0", "0.2.1"))
    }
}
