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
}
