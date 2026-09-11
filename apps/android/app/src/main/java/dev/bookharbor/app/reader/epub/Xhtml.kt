package dev.bookharbor.app.reader.epub

import org.w3c.dom.Document
import org.w3c.dom.Element
import org.w3c.dom.Node
import org.xml.sax.InputSource
import java.io.InputStream
import java.io.StringReader
import javax.xml.parsers.DocumentBuilderFactory

internal object Xml {
    /** Parses untrusted XML: no DTD or external entity is ever fetched. */
    fun parse(stream: InputStream): Document {
        val factory = DocumentBuilderFactory.newInstance().apply {
            isNamespaceAware = true
            isValidating = false
            isExpandEntityReferences = false
        }
        val builder = factory.newDocumentBuilder()
        builder.setEntityResolver { _, _ -> InputSource(StringReader("")) }
        builder.setErrorHandler(null)
        return try {
            builder.parse(stream)
        } catch (error: Exception) {
            throw EpubException("Malformed XML in EPUB", error)
        }
    }

    fun children(parent: Node): List<Element> {
        val out = ArrayList<Element>()
        var node = parent.firstChild
        while (node != null) {
            if (node is Element) out += node
            node = node.nextSibling
        }
        return out
    }

}

/** Lower-case tag name without any namespace prefix. */
internal fun Element.tag(): String = (localName ?: nodeName.substringAfter(':')).lowercase()

/** Turns one chapter's XHTML into an ordered list of [Block]s. Scripts and styles are dropped. */
object XhtmlBlocks {
    private val HEADINGS = setOf("h1", "h2", "h3", "h4", "h5", "h6")
    private val CONTAINERS = setOf("div", "section", "article", "main", "aside", "nav", "figure", "header", "footer", "body", "ol", "ul", "dl", "table", "tbody", "thead", "tfoot", "tr", "details", "figcaption")
    private val SKIPPED = setOf("script", "style", "head", "svg", "math", "audio", "video", "object", "iframe", "canvas")
    private val PARAGRAPHS = setOf("p", "li", "dt", "dd", "td", "th", "caption", "address", "summary")

    fun parse(stream: InputStream): List<Block> {
        val body = Xml.children(Xml.parse(stream).documentElement).firstOrNull { it.tag() == "body" }
            ?: return emptyList()
        val blocks = ArrayList<Block>()
        walk(body, "/4", blocks)
        return blocks
    }

    private fun walk(parent: Element, parentPath: String, out: MutableList<Block>) {
        Xml.children(parent).forEachIndexed { index, element ->
            val path = "$parentPath/${2 * (index + 1)}"
            val name = element.tag()
            when {
                name in SKIPPED -> Unit
                name in HEADINGS -> text(element).takeIf { it.isNotEmpty() }?.let { out += Block.Heading(name.last() - '0', it, path) }
                name == "hr" -> out += Block.Rule(path)
                name == "img" -> image(element, path)?.let { out += it }
                name == "pre" -> out += Block.Preformatted(element.textContent.trim('\n'), path)
                name == "blockquote" -> leaf(element, path, out) { Block.Quote(it, path) }
                name in PARAGRAPHS -> leaf(element, path, out) { Block.Paragraph(if (name == "li") "• $it" else it, path) }
                name in CONTAINERS -> walk(element, path, out)
                else -> leaf(element, path, out) { Block.Paragraph(it, path) } // inline content sitting directly in a container
            }
        }
    }

    /**
     * A textual block. If it also holds block children (a blockquote of paragraphs, a list item
     * with a nested list) they are walked instead so each gets its own position.
     */
    private fun leaf(element: Element, path: String, out: MutableList<Block>, make: (String) -> Block) {
        if (hasBlockChild(element)) { walk(element, path, out); return }
        text(element).takeIf { it.isNotEmpty() }?.let { out += make(it) }
        images(element, path).forEach { out += it }
    }

    private fun hasBlockChild(element: Element) = Xml.children(element).any {
        it.tag() in HEADINGS || it.tag() in CONTAINERS || it.tag() in PARAGRAPHS || it.tag() == "blockquote" || it.tag() == "pre"
    }

    private fun images(element: Element, path: String): List<Block.Image> =
        Xml.children(element).flatMapIndexed { index, child ->
            val childPath = "$path/${2 * (index + 1)}"
            if (child.tag() == "img") listOfNotNull(image(child, childPath)) else images(child, childPath)
        }

    private fun image(element: Element, path: String): Block.Image? {
        val src = element.getAttribute("src").ifBlank { element.getAttributeNS("http://www.w3.org/1999/xlink", "href") }
        return if (src.isBlank()) null else Block.Image(src, element.getAttribute("alt"), path)
    }

    /** Visible text with runs of whitespace collapsed; `<br>` becomes a line break. */
    private fun text(element: Element): String {
        val builder = StringBuilder()
        collect(element, builder)
        return builder.toString().replace(Regex("[ \\t\\r\\f\\u00a0]*\\n[ \\t\\r\\f\\u00a0]*"), "\n").replace(Regex("[ \\t\\r\\f\\u00a0]+"), " ").trim()
    }

    private fun collect(node: Node, out: StringBuilder) {
        var child = node.firstChild
        while (child != null) {
            when (child) {
                is Element -> {
                    val name = child.tag()
                    when {
                        name in SKIPPED || name == "img" -> Unit
                        name == "br" -> out.append('\n')
                        else -> collect(child, out)
                    }
                }
                else -> if (child.nodeType == Node.TEXT_NODE || child.nodeType == Node.CDATA_SECTION_NODE) out.append(child.nodeValue.replace('\n', ' '))
            }
            child = child.nextSibling
        }
    }
}
