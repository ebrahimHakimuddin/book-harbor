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

    @Test fun appAndServerMustShareMajorAndMinorVersion() {
        assertTrue(compatibleVersions("1.0.0", "1.0.4"))
        assertFalse(compatibleVersions("1.1.0", "1.0.0"))
        assertFalse(compatibleVersions("1.0.0", "2.0.0"))
        assertTrue(compatibleVersions("dev", "1.0.0"))
        assertTrue(compatibleVersions("1.0.0", ""))
        assertTrue(versionMismatchMessage("1.1.0", "1.0.0").contains("Update the app"))
        assertTrue(versionMismatchMessage("1.0.0", "1.1.0").contains("update it"))
    }
}
