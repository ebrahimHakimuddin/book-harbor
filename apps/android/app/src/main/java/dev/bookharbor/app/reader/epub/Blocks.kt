package dev.bookharbor.app.reader.epub

/**
 * The renderable structure of one EPUB chapter. Every block remembers [path], its position in
 * the document as CFI steps ("/4/2/6" = body › 1st child › 2nd… in CFI even-index notation), so
 * a saved reading position resolves to the same passage after a reflow or restart.
 */
/** A `<a href>` found in a text block's content, as a range into that block's [Block] text. */
data class LinkSpan(val start: Int, val end: Int, val href: String)

sealed interface Block {
    val path: String
    /** The element's `id` attribute, if any -- how another block's in-book hyperlink can target it. */
    val id: String

    data class Heading(val level: Int, val text: String, override val path: String, override val id: String = "", val links: List<LinkSpan> = emptyList()) : Block
    data class Paragraph(val text: String, override val path: String, override val id: String = "", val links: List<LinkSpan> = emptyList()) : Block
    data class Quote(val text: String, override val path: String, override val id: String = "", val links: List<LinkSpan> = emptyList()) : Block
    data class Preformatted(val text: String, override val path: String, override val id: String = "") : Block
    data class Image(val href: String, val alt: String, override val path: String, override val id: String = "") : Block
    data class Rule(override val path: String, override val id: String = "") : Block
}

data class Chapter(
    /** Manifest id; stable across sessions, used to key per-chapter progress. */
    val id: String,
    /** Archive path of the XHTML document. */
    val href: String,
    val title: String,
    /** Uncompressed size, used to weight overall progress by how much text each chapter holds. */
    val weight: Long,
)

class EpubException(message: String, cause: Throwable? = null) : Exception(message, cause)
