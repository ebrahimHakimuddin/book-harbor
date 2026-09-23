package dev.bookharbor.app.library

enum class DownloadStatus { NOT_DOWNLOADED, DOWNLOADING, AVAILABLE, FAILED }

enum class ShelfFilter(val label: String) { All("All"), Reading("Reading"), Finished("Finished"), Downloaded("Downloaded") }

enum class BookSort(val label: String) { Recent("Recently added"), Title("Title A–Z"), Author("Author A–Z"), Series("Series") }

/** How far through a book counts as finished. */
const val FINISHED_AT = 0.97

/** 0 < progress < FINISHED_AT. */
fun isReading(progress: Double?) = progress != null && progress > 0.0 && progress < FINISHED_AT
fun isFinished(progress: Double?) = progress != null && progress >= FINISHED_AT

/** Picks the edition to open: a downloaded one first, and EPUB before PDF since it reflows. */
fun preferredEdition(book: Book, downloaded: (Edition) -> Boolean): Edition? =
    book.editions.sortedWith(compareByDescending<Edition> { downloaded(it) }.thenBy { if (it.format == "epub") 0 else 1 }).firstOrNull()

/**
 * The books to show for the search box, shelf chip, and sort order. [progress] maps book ID to
 * 0..1 reading progress; [downloadedEditions] holds edition IDs available offline.
 */
fun visibleBooks(
    books: List<Book>,
    query: String,
    filter: ShelfFilter,
    sort: BookSort,
    progress: Map<String, Double>,
    downloadedEditions: Set<String>,
    /** Only books carrying this tag, or every book when null. */
    tag: String? = null,
): List<Book> {
    val needle = query.trim().lowercase()
    val matching = books.filter { book ->
        val textMatch = needle.isEmpty() || (listOf(book.title, book.subtitle, book.series) + book.authors + book.tags).any { it.lowercase().contains(needle) }
        val tagMatch = tag == null || book.tags.any { it.equals(tag, ignoreCase = true) }
        val shelfMatch = when (filter) {
            ShelfFilter.All -> true
            ShelfFilter.Reading -> isReading(progress[book.id])
            ShelfFilter.Finished -> isFinished(progress[book.id])
            ShelfFilter.Downloaded -> book.editions.any { it.id in downloadedEditions }
        }
        textMatch && shelfMatch && tagMatch
    }
    return when (sort) {
        BookSort.Recent -> matching // the server already returns newest first
        BookSort.Title -> matching.sortedBy { it.title.lowercase() }
        BookSort.Author -> matching.sortedWith(compareBy({ it.authors.firstOrNull().isNullOrBlank() }, { it.authors.firstOrNull()?.lowercase().orEmpty() }, { it.title.lowercase() }))
        // Series together and in reading order; standalone books after them by title.
        BookSort.Series -> matching.sortedWith(compareBy({ it.series.isBlank() }, { it.series.lowercase() }, { it.seriesIndex }, { it.title.lowercase() }))
    }
}

/** Every tag in the library, most used first, for the tag filter row. */
fun libraryTags(books: List<Book>): List<String> =
    books.flatMap { it.tags }.groupBy { it.lowercase() }.values.sortedWith(compareByDescending<List<String>> { it.size }.thenBy { it.first().lowercase() }).map { it.first() }

/** "Harbor Cycle #2", "Harbor Cycle #2.5", or just the series name when unnumbered. */
fun seriesLabel(book: Book): String = when {
    book.series.isBlank() -> ""
    book.seriesIndex <= 0.0 -> book.series
    book.seriesIndex % 1.0 == 0.0 -> "${book.series} #${book.seriesIndex.toLong()}"
    else -> "${book.series} #${book.seriesIndex}"
}

/**
 * Home holds the books the reader is actually using: anything downloaded to this device or
 * started (on any device). Everything else lives in Browse, the server's whole catalogue.
 */
fun isOnShelf(book: Book, progress: Map<String, Double>, downloadedEditions: Set<String>): Boolean =
    (progress[book.id] ?: 0.0) > 0.0 || book.editions.any { it.id in downloadedEditions }

enum class HomeSection(val title: String) { Reading("Reading"), UpNext("Up next"), Finished("Finished") }

/**
 * Home's shelf in sections: books in progress, downloaded books not started yet, and finished
 * ones, keeping [books]' order within each. Empty sections are left out.
 */
fun homeSections(books: List<Book>, progress: Map<String, Double>, downloadedEditions: Set<String>): List<Pair<HomeSection, List<Book>>> {
    val grouped = books.groupBy { book ->
        val p = progress[book.id]
        when {
            isFinished(p) -> HomeSection.Finished
            isReading(p) -> HomeSection.Reading
            book.editions.any { it.id in downloadedEditions } -> HomeSection.UpNext
            else -> null
        }
    }
    return HomeSection.entries.mapNotNull { section -> grouped[section]?.takeIf { it.isNotEmpty() }?.let { section to it } }
}

/** The catalogue's newest books first (the server's order), for Browse's "New arrivals" row. */
fun newArrivals(books: List<Book>, count: Int = 12): List<Book> = books.take(count)

/** Each series with its books in reading order, series with the most books first. */
fun seriesGroups(books: List<Book>): List<Pair<String, List<Book>>> =
    books.filter { it.series.isNotBlank() }.groupBy { it.series }
        .map { (name, members) -> name to members.sortedWith(compareBy({ it.seriesIndex }, { it.title.lowercase() })) }
        .sortedWith(compareByDescending<Pair<String, List<Book>>> { it.second.size }.thenBy { it.first.lowercase() })

/** The in-progress book read most recently, for the library's "Continue reading" card. */
fun continueReading(books: List<Book>, progress: Map<String, Double>, lastReadAt: Map<String, String>): Book? =
    books.filter { isReading(progress[it.id]) }
        .maxByOrNull { book -> lastReadAt[book.id]?.let { runCatching { java.time.Instant.parse(it) }.getOrNull() } ?: java.time.Instant.EPOCH }

fun initialsOf(name: String): String = name.split(Regex("\\s+")).filter { it.isNotBlank() }.take(2).joinToString("") { it.first().uppercase() }.ifEmpty { "BH" }
