package dev.bookharbor.app.reader.epub

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

class EpubBookTest {
    private fun epub(vararg extra: Pair<String, String>, opf: String? = null, nav: String? = NAV, ncx: String? = null, chapters: Map<String, String> = CHAPTERS): File {
        val file = Files.createTempFile("book", ".epub").toFile()
        ZipOutputStream(file.outputStream()).use { zip ->
            fun put(name: String, text: String, stored: Boolean = false) { zip.putNextEntry(ZipEntry(name)); zip.write(text.toByteArray()); zip.closeEntry() }
            put("mimetype", "application/epub+zip")
            put("META-INF/container.xml", """<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>""")
            put("OEBPS/content.opf", opf ?: OPF)
            nav?.let { put("OEBPS/nav.xhtml", it) }
            ncx?.let { put("OEBPS/toc.ncx", it) }
            chapters.forEach { (name, body) -> put("OEBPS/$name", body) }
            extra.forEach { (name, body) -> put(name, body) }
        }
        return file
    }

    @Test fun readsSpineTitlesAndWeights() {
        EpubBook.open(epub()).use { book ->
            assertEquals("The Test Harbor", book.title)
            assertEquals(listOf("The Map Room", "The Sounding Line"), book.chapters.map { it.title })
            assertTrue(book.chapters.all { it.weight > 0 })
        }
    }

    @Test fun fallsBackToTheNcxAndThenToChapterNumbers() {
        val withNcx = epub(nav = null, ncx = """<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/"><navMap><navPoint><navLabel><text>Old Style One</text></navLabel><content src="ch1.xhtml"/></navPoint></navMap></ncx>""",
            opf = OPF.replace("""<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>""", """<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>""").replace("<spine>", """<spine toc="ncx">"""))
        EpubBook.open(withNcx).use { assertEquals(listOf("Old Style One", "Chapter 2"), it.chapters.map { c -> c.title }) }
    }

    @Test fun convertsXhtmlToBlocksAndDropsActiveContent() {
        EpubBook.open(epub()).use { book ->
            val blocks = book.blocks(0)
            assertEquals(listOf("Heading", "Paragraph", "Quote", "Paragraph", "Image", "Rule"), blocks.map { it::class.simpleName })
            assertEquals("Chapter One", (blocks[0] as Block.Heading).text)
            assertEquals("Harbor light, then dawn.", (blocks[1] as Block.Paragraph).text)
            assertTrue(blocks.none { it is Block.Paragraph && it.text.contains("alert") }) // <script> removed
            assertEquals("• first", (blocks[3] as Block.Paragraph).text)
            assertEquals("pic.png", (blocks[4] as Block.Image).href)
        }
    }

    @Test fun blockPathsAreRealCfiSteps() {
        EpubBook.open(epub()).use { book ->
            assertEquals(listOf("/4/2", "/4/4", "/4/6", "/4/8/2", "/4/10/2", "/4/12"), book.blocks(0).map { it.path })
        }
    }

    @Test fun savedPositionsRoundTripAndResolveToTheSameBlock() {
        val position = EpubPosition(1, "/4/6/2", 0)
        assertEquals("epubcfi(/6/4!/4/6/2:0)", position.toCfi())
        assertEquals(position, EpubPosition.parse(position.toCfi()))
        assertNull(EpubPosition.parse("epubcfi(/6/3!/4/2)")) // odd spine step is not a chapter
        assertNull(EpubPosition.parse("not a cfi"))

        EpubBook.open(epub()).use { book ->
            val blocks = book.blocks(0)
            assertEquals(2, blocks.indexOfPath("/4/6"))
            assertEquals(3, blocks.indexOfPath("/4/8/2"))
            assertEquals(3, blocks.indexOfPath("/4/8/99")) // vanished child: nearest sibling context
            assertEquals(0, emptyList<Block>().indexOfPath("/4/2"))
        }
    }

    @Test fun overallProgressIsWeightedByChapterSize() {
        EpubBook.open(epub()).use { book ->
            assertEquals(0.0, book.overallProgress(0, 0.0), 1e-9)
            assertEquals(1.0, book.overallProgress(1, 1.0), 1e-9)
            val firstShare = book.chapters[0].weight.toDouble() / book.totalWeight
            assertEquals(firstShare, book.overallProgress(1, 0.0), 1e-9)
            assertTrue(book.overallProgress(0, 0.5) < firstShare)
        }
    }

    @Test fun resourcesResolveRelativeToTheChapterAndCannotEscapeTheArchive() {
        val file = epub("OEBPS/images/pic.png" to "PNGDATA", "secret.txt" to "outside")
        EpubBook.open(file).use { book ->
            assertEquals("PNGDATA", String(book.resource("OEBPS/ch1.xhtml", "images/pic.png")!!))
            assertNull(book.resource("OEBPS/ch1.xhtml", "../../etc/passwd"))
            assertNull(book.resource("OEBPS/ch1.xhtml", "https://example.com/x.png"))
            assertNull(book.resource("OEBPS/ch1.xhtml", "images/missing.png"))
        }
    }

    @Test fun decodesPercentEncodedHrefs() {
        assertEquals("OEBPS/my chapter.xhtml", EpubBook.resolve("OEBPS/content.opf", "my%20chapter.xhtml"))
        assertEquals("OEBPS/a/b.xhtml", EpubBook.resolve("OEBPS/nav.xhtml", "./a/x/../b.xhtml#frag"))
        assertNull(EpubBook.resolve("OEBPS/nav.xhtml", "../../out.xhtml"))
    }

    @Test fun rejectsBrokenInput() {
        assertThrows(EpubException::class.java) { EpubBook.open(Files.createTempFile("x", ".epub").toFile().apply { writeText("not a zip") }) }
        assertThrows(EpubException::class.java) { EpubBook.open(epub(opf = "<package")) }
    }

    @Test fun doesNotFetchExternalEntities() {
        // A hostile chapter declaring an external entity must parse without touching the file or network.
        val hostile = """<?xml version="1.0"?><!DOCTYPE html [<!ENTITY x SYSTEM "file:///etc/passwd">]><html xmlns="http://www.w3.org/1999/xhtml"><body><p>safe &x;</p></body></html>"""
        EpubBook.open(epub(chapters = mapOf("ch1.xhtml" to hostile, "ch2.xhtml" to CH2))).use { book ->
            val text = book.blocks(0).filterIsInstance<Block.Paragraph>().joinToString { it.text }
            assertTrue(text, !text.contains("root:"))
        }
    }

    private companion object {
        const val OPF = """<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">t</dc:identifier><dc:title>The Test Harbor</dc:title></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="c1" href="ch1.xhtml" media-type="application/xhtml+xml"/><item id="c2" href="ch2.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/><itemref idref="c2"/></spine></package>"""
        const val NAV = """<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol><li><a href="ch1.xhtml">The Map Room</a></li><li><a href="ch2.xhtml#s1">The Sounding Line</a></li></ol></nav></body></html>"""
        const val CH1 = """<html xmlns="http://www.w3.org/1999/xhtml"><head><title>x</title><style>p{}</style></head><body><h1>Chapter One</h1><p>Harbor   light,
            then <em>dawn</em>.</p><blockquote>A quoted line.</blockquote><ul><li>first</li></ul><p><img src="pic.png" alt="Pic"/></p><hr/><script>alert(1)</script></body></html>"""
        const val CH2 = """<html xmlns="http://www.w3.org/1999/xhtml"><body><h2>Two</h2><p>Second chapter text that is somewhat longer than the first one is.</p></body></html>"""
        val CHAPTERS = mapOf("ch1.xhtml" to CH1, "ch2.xhtml" to CH2)
    }
}
