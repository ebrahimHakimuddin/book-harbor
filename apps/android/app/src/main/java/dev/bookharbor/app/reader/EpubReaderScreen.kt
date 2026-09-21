package dev.bookharbor.app.reader

import android.content.Intent
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.activity.compose.BackHandler
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import dev.bookharbor.app.reader.epub.Block
import dev.bookharbor.app.reader.epub.EpubBook
import dev.bookharbor.app.reader.epub.EpubPosition
import dev.bookharbor.app.reader.epub.LinkSpan
import dev.bookharbor.app.reader.epub.indexOfPath
import dev.bookharbor.app.sync.LocalPosition
import dev.bookharbor.app.sync.Locator
import dev.bookharbor.app.sync.ProgressRecorder
import dev.bookharbor.app.ui.theme.InterFamily
import dev.bookharbor.app.ui.theme.LiterataFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlin.math.roundToInt

private const val HEADER_ITEMS = 1

@OptIn(FlowPreview::class)
@Composable
fun EpubReaderScreen(
    book: EpubBook,
    bookId: String,
    editionId: String,
    bookTitle: String,
    restore: LocalPosition?,
    recorder: ProgressRecorder,
    settingsStore: ReaderSettingsStore,
    onClose: () -> Unit,
) {
    val restored = remember { restore?.locator?.takeIf { it.kind == Locator.EPUB }?.let { EpubPosition.parse(it.value) }?.takeIf { it.chapterIndex in book.chapters.indices } }
    var state by remember {
        mutableStateOf(
            ReaderState(
                bookTitle = bookTitle.ifBlank { book.title },
                chapters = book.chapters.map { ReaderChapter(it.id, it.title, it.weight) },
                chapterIndex = restored?.chapterIndex ?: 0,
                settings = settingsStore.load(),
            ),
        )
    }
    val dispatch: (ReaderAction) -> Unit = { state = state.reduce(it) }
    LaunchedEffect(state.settings) { settingsStore.save(state.settings) }

    val chapterIndex = state.chapterIndex
    val blocks by produceState<List<Block>?>(null, chapterIndex) {
        value = null
        value = withContext(Dispatchers.IO) { runCatching { book.blocks(chapterIndex) }.getOrDefault(emptyList()) }
    }
    val listState = rememberLazyListState()
    var pendingRestore by remember { mutableStateOf(restored) }
    var ready by remember(chapterIndex) { mutableStateOf(false) }

    // Position the list once a chapter's blocks exist: restore the saved passage, else the start.
    LaunchedEffect(chapterIndex, blocks) {
        val loaded = blocks ?: return@LaunchedEffect
        val target = pendingRestore?.takeIf { it.chapterIndex == chapterIndex }
        val index = target?.let { loaded.indexOfPath(it.path) + HEADER_ITEMS } ?: 0
        pendingRestore = null
        listState.scrollToItem(index)
        ready = true
    }

    val currentBlocks = rememberUpdatedState(blocks)
    // Keyed on chapterIndex: a chapter switch (TOC jump, next-chapter) must not let a flush
    // right after landing persist the old chapter's block index against the new chapter's blocks.
    var latest by remember(chapterIndex) { mutableStateOf<Pair<Int, Double>?>(null) }

    fun persist(position: Pair<Int, Double>) {
        val block = currentBlocks.value?.getOrNull(position.first) ?: return
        val cfi = EpubPosition(chapterIndex, block.path).toCfi()
        val percentage = book.overallProgress(chapterIndex, position.second)
        recorder.record(bookId, editionId, Locator.epub(cfi), percentage)
    }

    // A bounded cadence while scrolling; the flush below covers stopping and closing.
    LaunchedEffect(chapterIndex, ready, blocks) {
        val loaded = blocks
        if (!ready || loaded == null || loaded.isEmpty()) return@LaunchedEffect
        snapshotFlow { firstVisibleBlock(listState, loaded.size) }
            .distinctUntilChanged()
            .onEach { latest = it; dispatch(ReaderAction.RecordProgress(it.second.toFloat())) }
            .debounce(400)
            .collect { position -> withContext(Dispatchers.IO) { persist(position) } }
    }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { latest?.let(::persist) }
    // Keyed on chapterIndex, not Unit: a keyless DisposableEffect's onDispose closure is
    // captured once at first composition and never refreshed, so `persist` would keep closing
    // over that first chapter's index forever. Every chapter change must flush and reinstall
    // this effect so the closure that ultimately runs on unmount (screen close) always matches
    // the chapter the reader is actually on -- otherwise closing after changing chapters at all
    // (TOC jump, next-chapter, reaching a chapter boundary) re-persists the old chapter's
    // position right after closeAndFlush() wrote the correct one.
    DisposableEffect(chapterIndex) { onDispose { latest?.let(::persist) } }

    // Flush the debounced position before handing off — closing (either path) reads the
    // store synchronously and must not race the pending write.
    fun closeAndFlush() {
        latest?.let(::persist)
        onClose()
    }
    BackHandler(onBack = ::closeAndFlush)

    // Reaching the end of the last chapter records completion at 100% -- not whatever fraction
    // of the final chapter happened to be on screen -- then returns to the library.
    fun finishBook() {
        latest?.let { position ->
            currentBlocks.value?.getOrNull(position.first)?.let { block ->
                val cfi = EpubPosition(chapterIndex, block.path).toCfi()
                recorder.record(bookId, editionId, Locator.epub(cfi), 1.0)
            }
        }
        onClose()
    }

    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    fun openLink(href: String) {
        if (href.contains("://")) {
            runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(href))) }
            return
        }
        val targetHref = EpubBook.resolve(book.chapters[chapterIndex].href, href) ?: return
        val targetChapterIndex = book.chapters.indexOfFirst { it.href == targetHref }
        if (targetChapterIndex < 0) return
        val fragment = href.substringAfter('#', "")
        scope.launch(Dispatchers.IO) {
            // The anchor's own block, if this chapter's blocks are loaded and it has one;
            // otherwise the link just lands at the top of the target chapter.
            val targetPath = fragment.takeIf { it.isNotEmpty() }
                ?.let { frag -> runCatching { book.blocks(targetChapterIndex) }.getOrNull()?.firstOrNull { it.id == frag }?.path }
            withContext(Dispatchers.Main) {
                pendingRestore = targetPath?.let { EpubPosition(targetChapterIndex, it) }
                dispatch(ReaderAction.SelectChapter(targetChapterIndex))
            }
        }
    }

    ReaderScaffold(
        state = state,
        onAction = dispatch,
        onClose = ::closeAndFlush,
        progressLabel = "Chapter ${chapterIndex + 1} of ${state.chapters.size} · ${state.chapter.title}",
        progress = state.currentChapterProgress,
        percentage = (state.currentChapterProgress * 100).roundToInt(),
    ) { padding ->
        ChapterList(book, state, blocks, listState, padding, onLinkClick = ::openLink, onNextChapter = { dispatch(ReaderAction.NextChapter) }, onFinishBook = ::finishBook)
    }
}

/** Index of the first block that is clearly on screen, and how far through the chapter it is. */
private fun firstVisibleBlock(state: LazyListState, blockCount: Int): Pair<Int, Double> {
    val visible = state.layoutInfo.visibleItemsInfo
    val item = visible.firstOrNull { it.index >= HEADER_ITEMS && it.offset + it.size > 48 } ?: return 0 to 0.0
    val block = (item.index - HEADER_ITEMS).coerceIn(0, (blockCount - 1).coerceAtLeast(0))
    val atEnd = item.index >= blockCount + HEADER_ITEMS
    val fraction = if (atEnd) 1.0 else if (blockCount <= 1) 0.0 else block.toDouble() / (blockCount - 1)
    return block to fraction
}

@Composable
private fun ChapterList(
    book: EpubBook,
    state: ReaderState,
    blocks: List<Block>?,
    listState: LazyListState,
    padding: PaddingValues,
    onLinkClick: (String) -> Unit,
    onNextChapter: () -> Unit,
    onFinishBook: () -> Unit,
) {
    val settings = state.settings
    val fontSize = 18.sp * settings.fontScale
    val body = TextStyle(
        color = MaterialTheme.colorScheme.onBackground,
        fontFamily = if (settings.typeface == ReaderTypeface.Inter) InterFamily else LiterataFamily,
        fontSize = fontSize,
        lineHeight = fontSize * settings.lineHeight,
        textAlign = if (settings.alignment == ReaderAlignment.Justified) TextAlign.Justify else TextAlign.Start,
    )
    val gap = (14 * settings.paragraphSpacing).dp

    if (blocks == null) {
        Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        return
    }
    LazyColumn(state = listState, modifier = Modifier.fillMaxSize().padding(padding)) {
        item(key = "header") {
            Measure(settings) {
                Spacer(Modifier.height(30.dp))
                Text("CHAPTER ${state.chapterIndex + 1} OF ${state.chapters.size}", color = MaterialTheme.colorScheme.secondary, style = MaterialTheme.typography.labelMedium, letterSpacing = 1.7.sp)
                HorizontalDivider(Modifier.padding(top = 12.dp, bottom = 26.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.62f))
                Text(state.chapter.title, Modifier.semantics { heading() }, style = MaterialTheme.typography.headlineLarge)
                Spacer(Modifier.height(30.dp))
            }
        }
        itemsIndexed(blocks, key = { index, _ -> index }) { _, block ->
            Measure(settings) { BlockView(book, book.chapters[state.chapterIndex].href, block, body, gap, onLinkClick) }
        }
        item(key = "transition") {
            Measure(settings) {
                Spacer(Modifier.height(48.dp))
                ChapterTransition(state.nextChapter, onNextChapter, onFinishBook)
                Spacer(Modifier.height(64.dp))
            }
        }
    }
}

/** The reading column: capped width, centred, with the reader's chosen margin. */
@Composable
private fun Measure(settings: ReaderSettings, content: @Composable () -> Unit) {
    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.TopCenter) {
        Column(Modifier.widthIn(max = 680.dp).fillMaxWidth().padding(horizontal = settings.horizontalMargin.dp)) { content() }
    }
}

@Composable
private fun BlockView(book: EpubBook, chapterHref: String, block: Block, body: TextStyle, gap: androidx.compose.ui.unit.Dp, onLinkClick: (String) -> Unit) {
    when (block) {
        is Block.Heading -> Text(
            linkedText(block.text, block.links, onLinkClick),
            Modifier.padding(top = gap, bottom = gap / 2).semantics { heading() },
            style = when (block.level) { 1 -> MaterialTheme.typography.headlineLarge; 2 -> MaterialTheme.typography.headlineMedium; else -> MaterialTheme.typography.titleLarge },
        )
        is Block.Paragraph -> Text(linkedText(block.text, block.links, onLinkClick), Modifier.padding(bottom = gap), style = body)
        is Block.Quote -> Row(Modifier.padding(bottom = gap)) {
            Box(Modifier.width(3.dp).height(24.dp).background(MaterialTheme.colorScheme.secondary))
            Text(linkedText(block.text, block.links, onLinkClick), Modifier.padding(start = 14.dp), style = body.copy(fontStyle = FontStyle.Italic))
        }
        is Block.Preformatted -> Text(block.text, Modifier.padding(bottom = gap), style = body.copy(fontFamily = FontFamily.Monospace, fontSize = body.fontSize * 0.85, lineHeight = body.lineHeight * 0.9))
        is Block.Rule -> HorizontalDivider(Modifier.padding(vertical = gap), color = MaterialTheme.colorScheme.outline)
        is Block.Image -> BookImage(book, chapterHref, block, Modifier.padding(bottom = gap))
    }
}

/** [text] with each of [links] underlined and tappable, without disturbing plain (link-free) text. */
@Composable
private fun linkedText(text: String, links: List<LinkSpan>, onLinkClick: (String) -> Unit): AnnotatedString {
    if (links.isEmpty()) return AnnotatedString(text)
    val color = MaterialTheme.colorScheme.secondary
    return buildAnnotatedString {
        append(text)
        links.forEach { link ->
            if (link.start !in 0..text.length || link.end !in link.start..text.length) return@forEach
            addStyle(SpanStyle(color = color, textDecoration = TextDecoration.Underline), link.start, link.end)
            addLink(LinkAnnotation.Clickable(link.href) { onLinkClick(link.href) }, link.start, link.end)
        }
    }
}

@Composable
private fun BookImage(book: EpubBook, chapterHref: String, block: Block.Image, modifier: Modifier) {
    val bitmap by produceState<ImageBitmap?>(null, block.href) {
        value = withContext(Dispatchers.IO) {
            val bytes = book.resource(chapterHref, block.href) ?: return@withContext null
            val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
            BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
            if (bounds.outWidth <= 0) return@withContext null
            var sample = 1
            while (maxOf(bounds.outWidth, bounds.outHeight) / sample > 2048) sample *= 2 // bound memory for huge scans
            BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })?.asImageBitmap()
        }
    }
    val loaded = bitmap
    if (loaded != null) {
        Image(loaded, contentDescription = block.alt.ifBlank { null }, modifier = modifier.fillMaxWidth(), contentScale = ContentScale.FillWidth)
    } else if (block.alt.isNotBlank()) {
        Text("[${block.alt}]", modifier, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
