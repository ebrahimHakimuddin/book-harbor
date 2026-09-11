package dev.bookharbor.app.reader.epub

/**
 * Reading positions as EPUB canonical fragment identifiers:
 * `epubcfi(/6/{2*(chapter+1)}!{path}:{offset})`. `/6` is the package spine and each chapter
 * is an even step within it; the part after `!` walks the chapter's XHTML.
 */
data class EpubPosition(val chapterIndex: Int, val path: String, val offset: Int = 0) {
    fun toCfi(): String = "epubcfi(/6/${2 * (chapterIndex + 1)}!$path:$offset)"

    companion object {
        private val PATTERN = Regex("""^epubcfi\(/6/(\d+)!((?:/\d+)+)(?::(\d+))?\)$""")

        fun parse(cfi: String): EpubPosition? {
            val match = PATTERN.matchEntire(cfi.trim()) ?: return null
            val step = match.groupValues[1].toInt()
            if (step < 2 || step % 2 != 0) return null
            return EpubPosition(step / 2 - 1, match.groupValues[2], match.groupValues[3].ifEmpty { "0" }.toInt())
        }
    }
}

/**
 * Finds the block a saved [path] refers to. An exact match wins; otherwise the deepest block
 * sharing a path prefix, so a position inside a block that no longer exists still lands nearby.
 */
fun List<Block>.indexOfPath(path: String): Int {
    if (isEmpty()) return 0
    indexOfFirst { it.path == path }.let { if (it >= 0) return it }
    var best = 0
    var bestShared = -1
    val wanted = path.split('/')
    forEachIndexed { index, block ->
        val steps = block.path.split('/')
        var shared = 0
        while (shared < steps.size && shared < wanted.size && steps[shared] == wanted[shared]) shared++
        if (shared > bestShared) { bestShared = shared; best = index }
    }
    return best
}
