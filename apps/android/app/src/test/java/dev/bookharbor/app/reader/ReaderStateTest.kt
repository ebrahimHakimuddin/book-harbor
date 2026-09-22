package dev.bookharbor.app.reader

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ReaderStateTest {
    @Test
    fun progressIsClampedAndContributesToOverallBookProgress() {
        val state = ReaderState.preview().reduce(ReaderAction.RecordProgress(1.4f))

        assertEquals(1f, state.currentChapterProgress)
        assertEquals(33, state.overallPercentage)
    }

    @Test
    fun nextChapterCompletesCurrentChapterWithoutSkipping() {
        val initial = ReaderState.preview().reduce(ReaderAction.RecordProgress(0.7f))
        val next = initial.reduce(ReaderAction.NextChapter)

        assertEquals(1, next.chapterIndex)
        assertEquals(1f, next.chapterProgress["chapter-1"])
        assertEquals(0f, next.currentChapterProgress)
    }

    @Test
    fun settingsAreBoundedAndProgressCanBeHidden() {
        val state = ReaderState.preview()
            .reduce(ReaderAction.SetFontScale(4f))
            .reduce(ReaderAction.SetLineHeight(0.5f))
            .reduce(ReaderAction.SetMargin(2))
            .reduce(ReaderAction.SetProgressVisible(false))

        assertEquals(1.5f, state.settings.fontScale)
        assertEquals(1.25f, state.settings.lineHeight)
        assertEquals(16, state.settings.horizontalMargin)
        assertFalse(state.settings.showProgress)
    }

    @Test
    fun settingsSheetHasExplicitOpenAndCloseState() {
        val open = ReaderState.preview().reduce(ReaderAction.OpenSettings)
        assertTrue(open.settingsOpen)

        val closed = open.reduce(ReaderAction.CloseSettings)
        assertFalse(closed.settingsOpen)
    }

    @Test
    fun overallProgressIsWeightedByChapterSize() {
        val state = ReaderState(
            bookTitle = "B",
            chapters = listOf(ReaderChapter("a", "A", 100), ReaderChapter("b", "B", 300)),
        )
        assertEquals(0, state.overallPercentage)
        assertEquals(25, state.reduce(ReaderAction.NextChapter).overallPercentage) // finished the short chapter
        assertEquals(63, state.reduce(ReaderAction.NextChapter).reduce(ReaderAction.RecordProgress(0.5f)).overallPercentage) // 62.5 rounds up
    }

    @Test
    fun contentsJumpsToAnyChapterAndIgnoresInvalidOnes() {
        val open = ReaderState.preview().reduce(ReaderAction.OpenContents)
        assertTrue(open.contentsOpen)
        val jumped = open.reduce(ReaderAction.SelectChapter(2))
        assertEquals(2, jumped.chapterIndex)
        assertFalse(jumped.contentsOpen)
        assertEquals(2, jumped.reduce(ReaderAction.SelectChapter(9)).chapterIndex)
    }

    @Test
    fun settingsRoundTripAndTolerateCorruptStoredValues() {
        val custom = ReaderSettings(theme = ReaderTheme.Sepia, fontScale = 1.2f, alignment = ReaderAlignment.Justified, brightness = 0.4f, showProgress = false, volumeKeys = true, orientation = ReaderOrientation.Landscape, hyphenation = false)
        assertEquals(custom, ReaderSettings.fromMap(custom.toMap()))
        val corrupt = ReaderSettings.fromMap(mapOf("theme" to "Neon", "fontScale" to "huge", "margin" to "999", "brightness" to "9"))
        assertEquals(ReaderTheme.System, corrupt.theme)
        assertEquals(1f, corrupt.fontScale)
        assertEquals(48, corrupt.horizontalMargin)
        assertEquals(1f, corrupt.brightness)
    }

    @Test
    fun resetRestoresDefaults() {
        val changed = ReaderState.preview().reduce(ReaderAction.SelectTheme(ReaderTheme.Black)).reduce(ReaderAction.SetFontScale(1.4f))
        assertEquals(ReaderSettings.Default, changed.reduce(ReaderAction.ResetSettings).settings)
    }
}
