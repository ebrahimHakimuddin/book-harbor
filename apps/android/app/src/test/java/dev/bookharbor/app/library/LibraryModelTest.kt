package dev.bookharbor.app.library

import org.junit.Assert.assertEquals
import org.junit.Test

class LibraryModelTest {
    private fun edition(id: String, format: String) = Edition(id, format, "", "$id.$format", "/c/$id")
    private val dune = Book("b1", "Dune", listOf(edition("e1", "epub")), authors = listOf("Frank Herbert"))
    private val hail = Book("b2", "Project Hail Mary", listOf(edition("e2", "pdf"), edition("e3", "epub")), authors = listOf("Andy Weir"))
    private val anon = Book("b3", "Anonymous Tales", listOf(edition("e4", "pdf")))
    private val books = listOf(dune, hail, anon)
    private fun show(query: String = "", filter: ShelfFilter = ShelfFilter.All, sort: BookSort = BookSort.Recent, progress: Map<String, Double> = emptyMap(), downloaded: Set<String> = emptySet()) =
        visibleBooks(books, query, filter, sort, progress, downloaded).map { it.id }

    @Test fun searchMatchesTitleAndAuthorIgnoringCase() {
        assertEquals(listOf("b1"), show("HERB"))
        assertEquals(listOf("b2"), show("hail mary"))
        assertEquals(emptyList<String>(), show("zzz"))
    }

    @Test fun shelvesUseLocalProgressAndDownloads() {
        val progress = mapOf("b1" to 0.4, "b2" to 0.99)
        assertEquals(listOf("b1"), show(filter = ShelfFilter.Reading, progress = progress))
        assertEquals(listOf("b2"), show(filter = ShelfFilter.Finished, progress = progress))
        assertEquals(listOf("b3"), show(filter = ShelfFilter.Downloaded, downloaded = setOf("e4")))
        assertEquals(3, show(filter = ShelfFilter.All).size)
    }

    @Test fun sortsByTitleAndAuthorWithUnknownAuthorsLast() {
        assertEquals(listOf("b3", "b1", "b2"), show(sort = BookSort.Title))
        assertEquals(listOf("b2", "b1", "b3"), show(sort = BookSort.Author))
        assertEquals(listOf("b1", "b2", "b3"), show(sort = BookSort.Recent))
    }

    @Test fun seriesSortGroupsInReadingOrderAndTagsFilter() {
        val two = Book("s2", "Zeta", emptyList(), series = "Saga", seriesIndex = 2.0, tags = listOf("Fantasy"))
        val one = Book("s1", "Alpha", emptyList(), series = "Saga", seriesIndex = 1.0, tags = listOf("fantasy", "Sea"))
        val solo = Book("s3", "Beta", emptyList())
        val all = listOf(solo, two, one)
        assertEquals(listOf("s1", "s2", "s3"), visibleBooks(all, "", ShelfFilter.All, BookSort.Series, emptyMap(), emptySet()).map { it.id })
        assertEquals(listOf("s2", "s1"), visibleBooks(all, "", ShelfFilter.All, BookSort.Recent, emptyMap(), emptySet(), tag = "FANTASY").map { it.id })
        assertEquals(listOf("s2", "s1"), visibleBooks(all, "saga", ShelfFilter.All, BookSort.Recent, emptyMap(), emptySet()).map { it.id })
        assertEquals(listOf("Fantasy", "Sea"), libraryTags(all))
        assertEquals("Saga #2", seriesLabel(two))
        assertEquals("", seriesLabel(solo))
    }

    @Test fun homeHoldsDownloadedOrStartedBooksAndBrowseGroupsSeries() {
        assertEquals(true, isOnShelf(dune, mapOf("b1" to 0.2), emptySet()))
        assertEquals(true, isOnShelf(hail, emptyMap(), setOf("e3")))
        assertEquals(false, isOnShelf(anon, mapOf("b3" to 0.0), setOf("e1")))
        val a2 = Book("a2", "Two", emptyList(), series = "A", seriesIndex = 2.0)
        val a1 = Book("a1", "One", emptyList(), series = "A", seriesIndex = 1.0)
        val b1 = Book("x1", "Solo", emptyList(), series = "B", seriesIndex = 1.0)
        assertEquals(listOf("A" to listOf("a1", "a2"), "B" to listOf("x1")), seriesGroups(listOf(a2, b1, a1, anon)).map { (name, books) -> name to books.map { it.id } })
    }

    @Test fun chapterPreviewCentersNearTheCurrentChapterAndStaysInBounds() {
        assertEquals(listOf(0, 1, 2), previewWindow(3, 2, 5))
        assertEquals(listOf(0, 1, 2, 3, 4), previewWindow(150, 0, 5))
        assertEquals(listOf(40, 41, 42, 43, 44), previewWindow(150, 41, 5))
        assertEquals(listOf(145, 146, 147, 148, 149), previewWindow(150, 149, 5))
    }

    @Test fun homeSplitsTheShelfIntoReadingUpNextAndFinished() {
        val sections = homeSections(books, mapOf("b1" to 0.5, "b2" to 1.0), setOf("e4"))
        assertEquals(listOf(HomeSection.Reading to listOf("b1"), HomeSection.UpNext to listOf("b3"), HomeSection.Finished to listOf("b2")), sections.map { (s, b) -> s to b.map { it.id } })
        assertEquals(emptyList<Any>(), homeSections(books, emptyMap(), emptySet()))
    }

    @Test fun opensADownloadedEditionFirstThenPrefersEpub() {
        assertEquals("e3", preferredEdition(hail) { false }!!.id)
        assertEquals("e2", preferredEdition(hail) { it.id == "e2" }!!.id)
        assertEquals(null, preferredEdition(Book("x", "X", emptyList())) { true })
    }

    @Test fun catalogSurvivesTheOfflineCacheRoundTrip() {
        val full = Book("b9", "Cached", listOf(edition("e9", "epub").copy(byteLength = 12, sha256 = "ab")), "Sub", listOf("A", "B"), "/api/v1/books/b9/cover", "2026-01-01T00:00:00Z", "About it", "Saga", 2.5, listOf("Sci-fi"))
        val page = parseBookPage(encodeBooks(listOf(full)))
        assertEquals(listOf(full), page.books)
        assertEquals(null, page.nextCursor)
    }

    @Test fun initialsFromNames() {
        assertEquals("HM", initialsOf("Harbor Master"))
        assertEquals("M", initialsOf("mira"))
        assertEquals("BH", initialsOf("   "))
    }

    @Test
    fun continueReadingPicksTheMostRecentlyReadUnfinishedBook() {
        val progress = mapOf("b1" to 0.4, "b2" to 0.99)
        val lastRead = mapOf("b1" to "2026-09-20T10:00:00Z", "b2" to "2026-09-22T10:00:00Z")
        assertEquals(dune, continueReading(listOf(dune, hail), progress, lastRead)) // hail is finished
        assertEquals(null, continueReading(listOf(dune, hail), emptyMap(), lastRead))
    }
}
