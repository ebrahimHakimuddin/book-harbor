package dev.bookharbor.app.reader.epub

import org.w3c.dom.Element
import java.io.Closeable
import java.io.File
import java.net.URI
import java.net.URLDecoder
import java.util.zip.ZipFile

/** An opened EPUB: its spine as [chapters], and on-demand access to chapter blocks and resources. */
class EpubBook private constructor(
    private val zip: ZipFile,
    val title: String,
    val chapters: List<Chapter>,
) : Closeable {
    val totalWeight: Long = chapters.sumOf { it.weight }.coerceAtLeast(1)

    /** Blocks of one chapter. Only the chapter being read is ever held in memory. */
    fun blocks(chapterIndex: Int): List<Block> {
        val chapter = chapters.getOrNull(chapterIndex) ?: throw EpubException("No such chapter")
        return readEntry(chapter.href, MAX_DOCUMENT_BYTES).inputStream().use(XhtmlBlocks::parse)
    }

    /** Bytes of an image or other resource referenced from [fromHref], or null if absent/too large. */
    fun resource(fromHref: String, reference: String): ByteArray? {
        val target = resolve(fromHref, reference) ?: return null
        return runCatching { readEntry(target, MAX_RESOURCE_BYTES) }.getOrNull()
    }

    /** Overall progress 0..1 for a position: text volume before the chapter plus the share inside it. */
    fun overallProgress(chapterIndex: Int, fractionInChapter: Double): Double {
        val before = chapters.take(chapterIndex).sumOf { it.weight }
        val current = chapters.getOrNull(chapterIndex)?.weight ?: 0
        return ((before + current * fractionInChapter.coerceIn(0.0, 1.0)) / totalWeight).coerceIn(0.0, 1.0)
    }

    private fun readEntry(name: String, limit: Long): ByteArray {
        val entry = zip.getEntry(name) ?: throw EpubException("Missing $name")
        if (entry.size > limit) throw EpubException("$name is too large")
        // entry.size can be -1 or lie in a hostile archive, so the read itself is bounded too.
        zip.getInputStream(entry).use { input ->
            val out = java.io.ByteArrayOutputStream()
            val buffer = ByteArray(16 * 1024)
            var total = 0L
            while (true) {
                val count = input.read(buffer)
                if (count < 0) break
                total += count
                if (total > limit) throw EpubException("$name is too large")
                out.write(buffer, 0, count)
            }
            return out.toByteArray()
        }
    }

    override fun close() = zip.close()

    companion object {
        private const val MAX_DOCUMENT_BYTES = 8L shl 20
        private const val MAX_RESOURCE_BYTES = 24L shl 20

        fun open(file: File): EpubBook {
            val zip = try { ZipFile(file) } catch (error: Exception) { throw EpubException("Not a valid EPUB archive", error) }
            try {
                return parse(zip)
            } catch (error: Exception) {
                zip.close()
                throw if (error is EpubException) error else EpubException("Could not read this EPUB", error)
            }
        }

        private fun parse(zip: ZipFile): EpubBook {
            fun entry(name: String): java.io.InputStream = zip.getEntry(name)?.let(zip::getInputStream) ?: throw EpubException("Missing $name")
            val container = Xml.parse(entry("META-INF/container.xml"))
            val opfPath = container.getElementsByTagNameNS("*", "rootfile").let { nodes ->
                (0 until nodes.length).map { nodes.item(it) as Element }.firstOrNull { it.getAttribute("full-path").isNotBlank() }?.getAttribute("full-path")
            } ?: throw EpubException("EPUB has no package document")
            if (!safePath(opfPath)) throw EpubException("Unsafe package path")

            val opf = Xml.parse(entry(opfPath)).documentElement
            val parts = Xml.children(opf).associateBy { it.tag() }
            val title = parts["metadata"]?.let { metadata ->
                Xml.children(metadata).firstOrNull { it.tag() == "title" }?.textContent?.trim()
            }.orEmpty().ifBlank { "Untitled" }

            data class Item(val id: String, val href: String, val properties: String, val mediaType: String)
            val manifest = Xml.children(parts["manifest"] ?: throw EpubException("EPUB has no manifest")).filter { it.tag() == "item" }.mapNotNull {
                val href = resolve(opfPath, it.getAttribute("href")) ?: return@mapNotNull null
                Item(it.getAttribute("id"), href, it.getAttribute("properties"), it.getAttribute("media-type"))
            }.associateBy { it.id }

            val spine = Xml.children(parts["spine"] ?: throw EpubException("EPUB has no spine")).filter { it.tag() == "itemref" && it.getAttribute("linear") != "no" }
            val titles = tableOfContents(zip, opf, parts["spine"], manifest.values.map { Triple(it.id, it.href, it.properties + "|" + it.mediaType) })

            val chapters = spine.mapNotNull { ref ->
                val item = manifest[ref.getAttribute("idref")] ?: return@mapNotNull null
                val entry = zip.getEntry(item.href) ?: return@mapNotNull null
                if (!item.mediaType.contains("html")) return@mapNotNull null
                Chapter(item.id, item.href, titles[item.href].orEmpty(), entry.size.coerceAtLeast(1))
            }
            if (chapters.isEmpty()) throw EpubException("EPUB has no readable chapters")
            val numbered = chapters.mapIndexed { index, chapter -> if (chapter.title.isBlank()) chapter.copy(title = "Chapter ${index + 1}") else chapter }
            return EpubBook(zip, title, numbered)
        }

        /** href → title from the EPUB 3 nav document, falling back to the EPUB 2 NCX. */
        private fun tableOfContents(zip: ZipFile, opf: Element, spine: Element?, items: List<Triple<String, String, String>>): Map<String, String> {
            val titles = LinkedHashMap<String, String>()
            fun add(base: String, href: String, label: String) {
                val target = resolve(base, href.substringBefore('#')) ?: return
                if (label.isNotBlank()) titles.putIfAbsent(target, label.trim().replace(Regex("\\s+"), " "))
            }
            items.firstOrNull { it.third.contains("nav") && it.third.contains("html") }?.let { (_, href, _) ->
                runCatching {
                    val doc = zip.getEntry(href)?.let { zip.getInputStream(it).use(Xml::parse) } ?: return@runCatching
                    val navs = doc.getElementsByTagNameNS("*", "nav")
                    for (i in 0 until navs.length) {
                        val nav = navs.item(i) as Element
                        val type = nav.getAttributeNS("http://www.idpf.org/2007/ops", "type").ifEmpty { nav.getAttribute("epub:type") }
                        if (type.split(' ').contains("toc")) {
                            val anchors = nav.getElementsByTagNameNS("*", "a")
                            for (j in 0 until anchors.length) (anchors.item(j) as Element).let { add(href, it.getAttribute("href"), it.textContent) }
                        }
                    }
                }
            }
            if (titles.isEmpty()) {
                val ncxId = spine?.getAttribute("toc").orEmpty()
                items.firstOrNull { it.first == ncxId || it.third.contains("dtbncx") }?.let { (_, href, _) ->
                    runCatching {
                        val doc = zip.getEntry(href)?.let { zip.getInputStream(it).use(Xml::parse) } ?: return@runCatching
                        val points = doc.getElementsByTagNameNS("*", "navPoint")
                        for (i in 0 until points.length) {
                            val point = points.item(i) as Element
                            val label = point.getElementsByTagNameNS("*", "text").item(0)?.textContent.orEmpty()
                            val src = (point.getElementsByTagNameNS("*", "content").item(0) as? Element)?.getAttribute("src").orEmpty()
                            add(href, src, label)
                        }
                    }
                }
            }
            return titles
        }

        internal fun safePath(path: String) = path.isNotEmpty() && !path.startsWith("/") && !path.contains('\\') && path.split('/').none { it == ".." }

        /** Resolves [reference] against the archive path [base]; null when it would leave the archive. */
        internal fun resolve(base: String, reference: String): String? {
            if (reference.isBlank() || reference.contains("://") || reference.startsWith("data:")) return null
            val resolved = try {
                // hrefs are percent-encoded; zip entry names are not. Decode, then let URI normalise "./" and "../".
                val decoded = URLDecoder.decode(reference.substringBefore('#').replace("+", "%2B"), "UTF-8")
                URI("/" + base.substringBeforeLast('/', "").let { if (it.isEmpty()) "" else "$it/" }).resolve(URI(null, null, decoded, null)).path
            } catch (_: Exception) { return null }
            val path = resolved.removePrefix("/")
            return path.takeIf { safePath(it) }
        }
    }
}
