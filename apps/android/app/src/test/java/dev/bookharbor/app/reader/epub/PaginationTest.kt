package dev.bookharbor.app.reader.epub

import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PaginationTest {
    @Test
    fun reflowPreservesEveryCharacterAndFindsTheSavedPassage() = runBlocking {
        val blocks = listOf(
            Block.Heading(1, "Heading", "/4/2"),
            Block.Paragraph("A long chapter with many words. ".repeat(400), "/4/4"),
            Block.Quote("A quotation. ".repeat(80), "/4/6"),
        )
        for (charactersPerLine in listOf(12, 30, 60)) {
            val pages = paginateChapter(blocks, 100,
                fitText = { block, offset, available ->
                    val end = minOf(block.pageText()!!.length, offset + available / 20 * charactersPerLine)
                    PageFit(end, (end - offset + charactersPerLine - 1) / charactersPerLine * 20)
                }, otherHeight = { 10 }, gapAfter = { 10 })
            blocks.forEachIndexed { index, block ->
                val text = block.pageText()!!
                assertEquals(text, pages.flatMap { it.slices }.filter { it.blockIndex == index }
                    .joinToString("") { text.substring(it.start, it.end) })
            }
            val page = pages[pages.pageFor(1, 1500)]
            assertTrue(page.slices.any { it.blockIndex == 1 && 1500 in it.start until it.end })
        }
    }

    @Test
    fun tinyViewportDoesNotSplitEmojiCodePoints() = runBlocking {
        val text = "🌊🌅📚"
        val pages = paginateChapter(listOf(Block.Paragraph(text, "/4/2")), 1,
            { _, offset, _ -> PageFit(offset, 0) }, { 1 })
        assertEquals(3, pages.size)
        assertTrue(pages.all { it.first!!.end - it.first!!.start == 2 })
        assertEquals(text, pages.joinToString("") { text.substring(it.first!!.start, it.first!!.end) })
    }
    @Test
    fun longParagraphSplitsWithoutLosingTextAndRestoresOffset() = runBlocking {
        val text = "abcdefghijklmnopqrstuvwxyz".repeat(30)
        val blocks = listOf(Block.Paragraph(text, "/4/2"))
        val pages = paginateChapter(blocks, 30,
            fitText = { block, offset, available ->
                val end = (offset + (available / 10) * 12).coerceAtMost(block.pageText()!!.length)
                PageFit(end, ((end - offset + 11) / 12) * 10)
            }, otherHeight = { 10 })
        assertTrue(pages.size > 5)
        assertEquals(text, pages.flatMap { it.slices }.joinToString("") { text.substring(it.start, it.end) })
        val middle = pages[3].first!!
        assertEquals(3, pages.pageFor(middle.blockIndex, middle.start))
    }

    @Test
    fun nonTextBlocksRemainWholeAndEmptyChapterHasOnePage() = runBlocking {
        val blocks = listOf(Block.Paragraph("a".repeat(24), "/4/2"), Block.Image("a.jpg", "", "/4/4"), Block.Rule("/4/6"))
        val pages = paginateChapter(blocks, 20,
            fitText = { block, offset, available -> PageFit((offset + available).coerceAtMost(block.pageText()!!.length), available) },
            otherHeight = { if (it is Block.Image) 20 else 5 })
        assertTrue(pages.any { page -> page.slices.any { it.blockIndex == 1 } })
        assertEquals(1, paginateChapter(emptyList(), 20, { _, _, _ -> PageFit(0, 0) }, { 1 }).size)
    }
}
