package dev.bookharbor.app.reader

import dev.bookharbor.app.sync.Locator
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.LocalDate

class ReaderToolsTest {
    private val today = LocalDate.of(2026, 9, 22)

    @Test
    fun wordEmphasisBoldsTheStartOfEachWordButNotPunctuation() {
        val text = "A lighthouse, far off."
        val bold = emphasisRanges(text).map { text.substring(it.first, it.last + 1) }
        assertEquals(listOf("A", "ligh", "f", "o"), bold)
    }

    @Test
    fun longParagraphsSplitForSpeechAtSentenceBreaks() {
        val text = "The first sentence is right here now. " + "word ".repeat(30)
        val parts = chunks(text, 60)
        assertEquals("The first sentence is right here now.", parts.first())
        parts.forEach { assert(it.length <= 60) }
        assertEquals(text.split(" ").filter { it.isNotBlank() }, parts.flatMap { it.split(" ") }.filter { it.isNotBlank() })
    }

    @Test
    fun paceLearnsFromSteadyReadingAndIgnoresJumps() {
        assertNull(blendPace(2000.0, 500.0, 10.0)) // too short to mean anything
        assertNull(blendPace(2000.0, 100_000.0, 60.0)) // a jump, not reading
        assertEquals(2000.0 * 0.8 + 3000.0 * 0.2, blendPace(2000.0, 3000.0, 60.0)!!, 0.001)
        assertEquals("3 min left in chapter", minutesLeftLabel(5000.0, 2000.0))
        assertEquals("1 h 5 min left in chapter", minutesLeftLabel(130_000.0, 2000.0))
        assertNull(minutesLeftLabel(0.0, 2000.0))
    }

    @Test
    fun streakCountsBackFromTodayOrYesterday() {
        val days = setOf(today.minusDays(1), today.minusDays(2), today.minusDays(4))
        assertEquals(2, streak(days, today)) // not read yet today: yesterday's run still counts
        assertEquals(3, streak(days + today, today))
        assertEquals(0, streak(setOf(today.minusDays(3)), today))
    }

    @Test
    fun nightScheduleOverridesThemeAndBrightnessOnlyAtNight() {
        val settings = ReaderSettings(theme = ReaderTheme.Sepia, brightness = 0.8f, nightSchedule = true)
        assertEquals(settings, settings.effectiveAt(12))
        val night = settings.effectiveAt(23)
        assertEquals(ReaderTheme.Black, night.theme)
        assertEquals(settings.nightBrightness, night.brightness)
        assertEquals(ReaderTheme.Black, settings.effectiveAt(6).theme)
        assertEquals(ReaderTheme.Dark, settings.copy(theme = ReaderTheme.Dark).effectiveAt(2).theme) // keeps a dark choice
        assertEquals(settings.copy(nightSchedule = false), settings.copy(nightSchedule = false).effectiveAt(23))
    }

    @Test
    fun exportListsAnnotationsInReadingOrderNotInsertionOrder() {
        fun at(chapter: Int, path: String, excerpt: String) = Annotation(
            bookId = "b", kind = AnnotationKind.Highlight, locator = Locator.epub("epubcfi(/6/${2 * (chapter + 1)}!$path:0)"),
            label = "Ch ${chapter + 1}", excerpt = excerpt, createdAt = "",
        )
        val text = exportAnnotations("Book", listOf(at(1, "/4/2", "third"), at(0, "/4/10", "second"), at(0, "/4/2", "first")))
        assertEquals(listOf("first", "second", "third"), Regex("“(\\w+)”").findAll(text).map { it.groupValues[1] }.toList())
    }

    @Test
    fun snippetTrimsAroundTheMatch() {
        assertEquals("…llo wor…", snippetAround("hello world!", "o w", radius = 2))
        assertNull(snippetAround("hello", "xyz"))
    }

    @Test
    fun nightWindowCanWrapPastMidnightOrNot() {
        val wrap = ReaderSettings(nightSchedule = true, nightStart = 22, nightEnd = 6)
        assertEquals(listOf(true, true, false, false), listOf(23, 3, 6, 21).map(wrap::isNight))
        val sameDay = ReaderSettings(nightSchedule = true, nightStart = 1, nightEnd = 5)
        assertEquals(listOf(false, true, false), listOf(0, 1, 5).map(sameDay::isNight))
        assertEquals(false, ReaderSettings(nightSchedule = true, nightStart = 4, nightEnd = 4).isNight(4))
    }

    @Test
    fun highlightRangeCoversPassageOrWholeBlock() {
        fun highlight(offset: Int, end: Int) = Annotation(
            bookId = "b", kind = AnnotationKind.Highlight, locator = Locator.epub("epubcfi(/6/2!/4/2:$offset)"), label = "", endOffset = end, createdAt = "",
        )
        assertEquals(0 until 20, highlightRange(highlight(0, 0), 20))
        assertEquals(5 until 9, highlightRange(highlight(5, 9), 20))
        assertEquals(5 until 12, highlightRange(highlight(5, 40), 12)) // text got shorter: clamp, don't crash
    }
}
