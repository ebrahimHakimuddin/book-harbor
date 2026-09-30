package dev.bookharbor.app.reader.epub

import kotlinx.coroutines.yield

/** A half-open range into an original XHTML block. Page numbers are never saved as progress. */
data class PageSlice(val blockIndex: Int, val start: Int, val end: Int, val gapAfterPx: Int = 0)
data class ReadingPage(val slices: List<PageSlice>) {
    val first: PageSlice? get() = slices.firstOrNull()
}
data class PageFit(val end: Int, val heightPx: Int)

/**
 * Splits chapter blocks into measured pages. [fitText] returns the largest line-bounded text
 * prefix that fits [availableHeightPx], starting at [offset]. A zero-length fit starts a new
 * page; on an empty page we advance one code point to guarantee progress on tiny viewports.
 */
suspend fun paginateChapter(
    blocks: List<Block>,
    pageHeightPx: Int,
    fitText: (block: Block, offset: Int, availableHeightPx: Int) -> PageFit,
    otherHeight: (block: Block) -> Int,
    gapAfter: (block: Block) -> Int = { 0 },
): List<ReadingPage> {
    require(pageHeightPx > 0)
    val pages = ArrayList<ReadingPage>()
    val slices = ArrayList<PageSlice>()
    var used = 0
    fun flush() {
        if (slices.isNotEmpty()) {
            pages += ReadingPage(slices.toList())
            slices.clear()
            used = 0
        }
    }
    blocks.forEachIndexed { index, block ->
        yield()
        val text = block.pageText()
        if (text != null) {
            var offset = 0
            while (offset < text.length) {
                yield()
                val fit = fitText(block, offset, pageHeightPx - used)
                if (fit.end <= offset) {
                    if (slices.isNotEmpty()) { flush(); continue }
                    val next = if (Character.isHighSurrogate(text[offset]) && offset + 1 < text.length && Character.isLowSurrogate(text[offset + 1])) offset + 2 else offset + 1
                    slices += PageSlice(index, offset, next)
                    offset = next
                    flush()
                    continue
                }
                require(fit.end <= text.length && fit.heightPx >= 0)
                val gap = if (fit.end == text.length) gapAfter(block).coerceIn(0, (pageHeightPx - used - fit.heightPx).coerceAtLeast(0)) else 0
                slices += PageSlice(index, offset, fit.end, gap)
                used += fit.heightPx + gap
                offset = fit.end
                if (offset < text.length && used >= pageHeightPx) flush()
            }
        } else {
            val height = otherHeight(block).coerceIn(1, pageHeightPx)
            if (slices.isNotEmpty() && used + height > pageHeightPx) flush()
            val gap = gapAfter(block).coerceIn(0, pageHeightPx - used - height)
            slices += PageSlice(index, 0, 0, gap)
            used += height + gap
        }
    }
    flush()
    if (pages.isEmpty()) pages += ReadingPage(emptyList())
    return pages
}

fun Block.pageText(): String? = when (this) {
    is Block.Heading -> text
    is Block.Paragraph -> text
    is Block.Quote -> text
    is Block.Preformatted -> text
    is Block.Image, is Block.Rule -> null
}

/** Finds the page containing a saved CFI, including an offset within a long paragraph. */
fun List<ReadingPage>.pageFor(blockIndex: Int, offset: Int): Int {
    if (isEmpty()) return 0
    val exact = indexOfFirst { page -> page.slices.any { it.blockIndex == blockIndex && (it.end == 0 || offset in it.start until it.end) } }
    if (exact >= 0) return exact
    return indexOfLast { page -> page.first?.blockIndex?.let { it <= blockIndex } == true }.coerceAtLeast(0)
}
