package dev.bookharbor.app.library

enum class DownloadStatus { NOT_DOWNLOADED, DOWNLOADING, AVAILABLE, FAILED }

enum class ShelfFilter(val label: String) { All("All"), Reading("Reading"), Finished("Finished"), Downloaded("Downloaded") }

enum class BookSort(val label: String) { Recent("Recently added"), Title("Title A–Z"), Author("Author A–Z") }

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
): List<Book> {
    val needle = query.trim().lowercase()
    val matching = books.filter { book ->
        val textMatch = needle.isEmpty() || (listOf(book.title, book.subtitle) + book.authors).any { it.lowercase().contains(needle) }
        val shelfMatch = when (filter) {
            ShelfFilter.All -> true
            ShelfFilter.Reading -> isReading(progress[book.id])
            ShelfFilter.Finished -> isFinished(progress[book.id])
            ShelfFilter.Downloaded -> book.editions.any { it.id in downloadedEditions }
        }
        textMatch && shelfMatch
    }
    return when (sort) {
        BookSort.Recent -> matching // the server already returns newest first
        BookSort.Title -> matching.sortedBy { it.title.lowercase() }
        BookSort.Author -> matching.sortedWith(compareBy({ it.authors.firstOrNull().isNullOrBlank() }, { it.authors.firstOrNull()?.lowercase().orEmpty() }, { it.title.lowercase() }))
    }
}

fun initialsOf(name: String): String = name.split(Regex("\\s+")).filter { it.isNotBlank() }.take(2).joinToString("") { it.first().uppercase() }.ifEmpty { "BH" }
