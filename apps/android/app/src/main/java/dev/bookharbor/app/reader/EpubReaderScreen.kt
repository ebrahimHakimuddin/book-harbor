package dev.bookharbor.app.reader

import android.content.Intent
import android.app.SearchManager
import android.content.Context
import android.os.SystemClock
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.Animatable
import androidx.compose.ui.graphics.graphicsLayer
import kotlinx.coroutines.delay
import androidx.compose.animation.core.tween
import androidx.compose.foundation.gestures.animateScrollBy
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.Hyphens
import androidx.compose.ui.text.style.LineBreak
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
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.awaitLongPressOrCancellation
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.semantics.onLongClick
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
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
import androidx.compose.ui.unit.em
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
    annotationStore: AnnotationStore,
    stats: ReadingStats,
    onClose: () -> Unit,
    chapterMarks: ChapterMarks? = null,
    /** Called every minute while a web novel chapter still being fetched is on screen. */
    onPlaceholder: (suspend (chapterIndex: Int) -> Unit)? = null,
) {
    val annotations = remember { BookAnnotations(annotationStore, bookId, bookTitle.ifBlank { book.title }) }
    LaunchedEffect(Unit) { withContext(Dispatchers.IO) { annotations.load() } }
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
    // A web novel chapter the server hasn't fetched yet: keep asking for a newer copy, which
    // reopens the book here once this chapter has arrived.
    val placeholder = blocks?.any { it is Block.Paragraph && it.text.startsWith(WEBNOVEL_PLACEHOLDER) } == true
    if (placeholder && onPlaceholder != null) LaunchedEffect(chapterIndex) {
        while (true) {
            onPlaceholder(chapterIndex)
            delay(60_000)
        }
    }
    val listState = rememberLazyListState()
    var pendingRestore by remember { mutableStateOf(restored) }
    var ready by remember(chapterIndex) { mutableStateOf(false) }

    // Position the list once a chapter's blocks exist: restore the saved passage, else the start.
    LaunchedEffect(chapterIndex, blocks) {
        val loaded = blocks ?: return@LaunchedEffect
        val target = pendingRestore?.takeIf { it.chapterIndex == chapterIndex }
        val index = target?.takeUnless { it.isChapterStart }?.let { loaded.indexOfPath(it.path) + HEADER_ITEMS } ?: 0
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

    // Learns the reader's pace from steady forward reading: a sample spans at least 20 seconds,
    // and idle stretches, going back, or jumping ahead start a fresh one instead of skewing it.
    var paceMark by remember { mutableStateOf<Pair<Long, Double>?>(null) }
    fun samplePace(position: Pair<Int, Double>) {
        val now = SystemClock.elapsedRealtime()
        val bytes = book.overallProgress(chapterIndex, position.second) * book.totalWeight
        val mark = paceMark
        val seconds = mark?.let { (now - it.first) / 1000.0 } ?: 0.0
        when {
            mark == null || seconds > 600 || bytes < mark.second -> paceMark = now to bytes
            seconds >= 20 -> { stats.recordPace(bytes - mark.second, seconds); paceMark = now to bytes }
        }
    }

    // A bounded cadence while scrolling; the flush below covers stopping and closing.
    LaunchedEffect(chapterIndex, ready, blocks) {
        val loaded = blocks
        if (!ready || loaded == null || loaded.isEmpty()) return@LaunchedEffect
        snapshotFlow { firstVisibleBlock(listState, loaded.size) }
            .distinctUntilChanged()
            .onEach { latest = it; dispatch(ReaderAction.RecordProgress(it.second.toFloat())) }
            .debounce(400)
            .collect { position -> samplePace(position); withContext(Dispatchers.IO) { persist(position) } }
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
    // Finishing a chapter (its end card's button) marks it read in the chapter list.
    fun markFinished(index: Int) { chapterMarks?.set(bookId, listOf(index), true) }

    fun finishBook() {
        markFinished(chapterIndex)
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

    // Read aloud follows the text: the block being spoken is tinted and kept on screen.
    val speech = remember { ReadAloud(context) }
    DisposableEffect(Unit) { onDispose { speech.shutdown() } }
    LaunchedEffect(chapterIndex) { speech.stop() }
    LaunchedEffect(speech.speaking) {
        val index = (speech.speaking ?: return@LaunchedEffect) + HEADER_ITEMS
        if (listState.layoutInfo.visibleItemsInfo.none { it.index == index && it.offset >= 0 }) listState.animateScrollToItem(index)
    }
    val readAloud = ReadAloudControls(
        available = speech.available && !blocks.isNullOrEmpty(),
        playing = speech.speaking != null,
        paused = speech.paused,
        start = { blocks?.let { loaded -> speech.play(loaded.map { it.text() }, latest?.first ?: 0) } },
        togglePause = speech::togglePause,
        stop = speech::stop,
    )

    /** Scrolls one screen, less a line of overlap so the reader doesn't lose their place. */
    fun page(forward: Boolean) {
        val info = listState.layoutInfo
        val distance = (info.viewportEndOffset - info.viewportStartOffset) * 0.9f
        scope.launch { listState.animateScrollBy(if (forward) distance else -distance, tween(260)) }
    }
    val pace = remember { stats.bytesPerMinute() }

    /** Jumps to a block anywhere in the book: scrolls within this chapter, or swaps chapters and restores there. */
    fun goTo(position: EpubPosition) {
        if (position.chapterIndex !in book.chapters.indices) return
        if (position.chapterIndex == chapterIndex) {
            val loaded = currentBlocks.value ?: return
            scope.launch { listState.scrollToItem(loaded.indexOfPath(position.path) + HEADER_ITEMS) }
        } else {
            pendingRestore = position
            dispatch(ReaderAction.SelectChapter(position.chapterIndex))
        }
    }

    suspend fun search(query: String): List<SearchHit> = withContext(Dispatchers.IO) {
        val hits = ArrayList<SearchHit>()
        for (index in book.chapters.indices) {
            val chapterBlocks = runCatching { book.blocks(index) }.getOrNull() ?: continue
            for (block in chapterBlocks) {
                val snippet = block.text()?.let { snippetAround(it, query) } ?: continue
                hits += SearchHit(Locator.epub(EpubPosition(index, block.path).toCfi()), book.chapters[index].title, snippet)
                if (hits.size >= MAX_SEARCH_HITS) return@withContext hits
            }
        }
        hits
    }

    val here = latest?.let { position -> currentBlocks.value?.getOrNull(position.first) }?.let { block ->
        Locator.epub(EpubPosition(chapterIndex, block.path).toCfi()) to state.chapter.title
    }

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
                if (targetPath != null) goTo(EpubPosition(targetChapterIndex, targetPath))
                else if (targetChapterIndex != chapterIndex) dispatch(ReaderAction.SelectChapter(targetChapterIndex))
            }
        }
    }

    ReaderScaffold(
        state = state,
        onAction = dispatch,
        onClose = ::closeAndFlush,
        progressLabel = "Chapter ${chapterIndex + 1} of ${state.chapters.size} · " +
            (minutesLeftLabel(book.chapters[chapterIndex].weight * (1.0 - state.currentChapterProgress), pace) ?: state.chapter.title),
        progress = state.currentChapterProgress,
        percentage = (state.currentChapterProgress * 100).roundToInt(),
        isScrolling = listState.isScrollInProgress,
        annotations = annotations,
        here = here,
        onGoTo = { locator -> EpubPosition.parse(locator.value)?.let(::goTo) },
        search = ::search,
        stats = stats,
        onPage = ::page,
        readAloud = readAloud,
    ) { padding ->
        ChapterList(book, state, blocks, listState, padding, annotations, speaking = speech.speaking, onLinkClick = ::openLink, onNextChapter = { markFinished(chapterIndex); dispatch(ReaderAction.NextChapter) }, onFinishBook = ::finishBook)
    }
}

/** Index of the first block that is clearly on screen, and how far through the chapter it is. */
private fun firstVisibleBlock(state: LazyListState, blockCount: Int): Pair<Int, Double> {
    val visible = state.layoutInfo.visibleItemsInfo
    // Offsets count from the list's top edge, which sits under the floating top bar.
    val top = state.layoutInfo.beforeContentPadding + 48
    val item = visible.firstOrNull { it.index >= HEADER_ITEMS && it.offset + it.size > top } ?: return 0 to 0.0
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
    annotations: BookAnnotations,
    speaking: Int?,
    onLinkClick: (String) -> Unit,
    onNextChapter: () -> Unit,
    onFinishBook: () -> Unit,
) {
    val settings = state.settings
    val fontSize = 18.sp * settings.fontScale
    val textColor = MaterialTheme.colorScheme.onBackground
    // Remembered so every block's Text can skip recomposition while the reader scrolls.
    val body = remember(settings, textColor) { TextStyle(
        color = textColor,
        fontFamily = if (settings.typeface == ReaderTypeface.Inter) InterFamily else LiterataFamily,
        fontSize = fontSize,
        lineHeight = fontSize * settings.lineHeight,
        textAlign = if (settings.alignment == ReaderAlignment.Justified) TextAlign.Justify else TextAlign.Start,
        hyphens = if (settings.hyphenation) Hyphens.Auto else Hyphens.None,
        letterSpacing = settings.letterSpacing.em,
        lineBreak = LineBreak.Paragraph,
    ) }
    val gap = (14 * settings.paragraphSpacing).dp
    // Grouped once per change to the annotations, not on every scroll-driven recomposition.
    val highlightsByBlock = remember(annotations.items, state.chapterIndex) {
        annotations.items.filter { it.kind == AnnotationKind.Highlight }
            .mapNotNull { a -> EpubPosition.parse(a.locator.value)?.takeIf { it.chapterIndex == state.chapterIndex }?.let { it.path to a } }
            .groupBy({ it.first }, { it.second })
    }

    if (blocks == null) {
        // Most chapters load in a blink; only a slow one earns a spinner, so switching chapters
        // doesn't flash one.
        var slow by remember { mutableStateOf(false) }
        LaunchedEffect(Unit) { delay(250); slow = true }
        Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) { if (slow) CircularProgressIndicator() }
        return
    }
    val appear = remember(state.chapterIndex) { Animatable(0f) }
    LaunchedEffect(state.chapterIndex) { appear.animateTo(1f, tween(220)) }
    CompositionLocalProvider(LocalTextAids provides TextAids(settings.wordEmphasis, settings.wordSpacing)) {
    LazyColumn(state = listState, modifier = Modifier.fillMaxSize().graphicsLayer { alpha = appear.value }, contentPadding = padding) {
        item(key = "header") {
            Measure(settings) {
                Spacer(Modifier.height(30.dp))
                Text("CHAPTER ${state.chapterIndex + 1} OF ${state.chapters.size}", color = MaterialTheme.colorScheme.secondary, style = MaterialTheme.typography.labelMedium, letterSpacing = 1.7.sp)
                HorizontalDivider(Modifier.padding(top = 12.dp, bottom = 26.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.62f))
                Text(state.chapter.title, Modifier.semantics { heading() }, style = MaterialTheme.typography.headlineLarge)
                Spacer(Modifier.height(30.dp))
            }
        }
        // This chapter's highlights, grouped by the block they sit in.
        val highlights = highlightsByBlock
        itemsIndexed(blocks, key = { index, _ -> index }) { index, block ->
            // Most books open each chapter with its own title, which the header above already
            // shows. That block stays in the list (saved positions index into it) but draws nothing.
            if (index == 0 && block is Block.Heading && sameTitle(block.text, state.chapter.title)) return@itemsIndexed
            val spoken by animateColorAsState(if (index == speaking) MaterialTheme.colorScheme.secondary.copy(alpha = 0.10f) else Color.Transparent, tween(300), label = "spoken")
            Measure(settings, Modifier.background(spoken)) {
                val text = block.text()
                val href = book.chapters[state.chapterIndex].href
                if (text == null) BlockView(book, href, block, body, gap, onLinkClick)
                else {
                    val marks = highlights[block.path].orEmpty()
                    HighlightableBlock(annotations, state.chapterIndex, block.path, state.chapter.title, text, marks) {
                        BlockView(book, href, block, body, gap, onLinkClick, marks.map { highlightRange(it, text.length) })
                    }
                }
            }
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
}

private fun sameTitle(a: String, b: String): Boolean {
    fun norm(value: String) = value.lowercase().filter { it.isLetterOrDigit() }
    return norm(a).isNotEmpty() && norm(a) == norm(b)
}

/** Text a reader can highlight or search, or null for images and rules. */
private fun Block.text(): String? = when (this) {
    is Block.Heading -> text
    is Block.Paragraph -> text
    is Block.Quote -> text
    is Block.Preformatted -> text
    is Block.Image, is Block.Rule -> null
}

/**
 * Press-and-hold (or TalkBack's long-press action) on a paragraph offers highlighting all of it
 * or a chosen passage, a note, copy, and share.
 */
@Composable
private fun HighlightableBlock(
    annotations: BookAnnotations,
    chapterIndex: Int,
    path: String,
    chapterTitle: String,
    text: String,
    highlights: List<Annotation>,
    content: @Composable () -> Unit,
) {
    val context = LocalContext.current
    val clipboard = LocalClipboardManager.current
    var menu by remember { mutableStateOf(false) }
    var editingNote by remember { mutableStateOf(false) }
    var choosingPassage by remember { mutableStateOf(false) }
    var lookingUp by remember { mutableStateOf(false) }
    fun highlight(range: IntRange): Annotation {
        val whole = range.first == 0 && range.last == text.lastIndex
        return annotations.add(
            AnnotationKind.Highlight, Locator.epub(EpubPosition(chapterIndex, path, if (whole) 0 else range.first).toCfi()), chapterTitle,
            text.substring(range.first, range.last + 1).take(4000), endOffset = if (whole) 0 else range.last + 1,
        )
    }
    Box(
        Modifier
            .semantics { onLongClick(label = "Highlight options") { menu = true; true } }
            .pointerInput(path) {
                awaitEachGesture {
                    val down = awaitFirstDown(requireUnconsumed = false)
                    val press = awaitLongPressOrCancellation(down.id) ?: return@awaitEachGesture
                    menu = true
                    press.consume()
                    // Swallow the rest of the gesture so lifting the finger doesn't also toggle the chrome.
                    do { val event = awaitPointerEvent(); event.changes.forEach { it.consume() } } while (event.changes.any { it.pressed })
                }
            },
    ) {
        content()
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            if (highlights.isEmpty()) {
                DropdownMenuItem(text = { Text("Highlight paragraph") }, onClick = { menu = false; highlight(text.indices) })
            } else {
                DropdownMenuItem(text = { Text(if (highlights.size == 1) "Remove highlight" else "Remove highlights") }, onClick = { menu = false; highlights.forEach(annotations::delete) })
            }
            DropdownMenuItem(text = { Text("Highlight a passage…") }, onClick = { menu = false; choosingPassage = true })
            DropdownMenuItem(text = { Text(if (highlights.firstOrNull()?.note.isNullOrBlank()) "Add note" else "Edit note") }, onClick = { menu = false; editingNote = true })
            DropdownMenuItem(text = { Text("Look up a word…") }, onClick = { menu = false; lookingUp = true })
            DropdownMenuItem(text = { Text("Copy") }, onClick = { menu = false; clipboard.setText(AnnotatedString(text)) })
            DropdownMenuItem(text = { Text("Share") }, onClick = { menu = false; shareText(context, quote(text, annotations.bookTitle, chapterTitle)) })
        }
    }
    if (editingNote) {
        NoteDialog(
            highlights.firstOrNull()?.note.orEmpty(),
            onSave = { note ->
                // A note always hangs off a highlight, so noting a plain paragraph highlights it too.
                annotations.setNote(highlights.firstOrNull() ?: highlight(text.indices), note)
                editingNote = false
            },
            onDismiss = { editingNote = false },
        )
    }
    if (choosingPassage) PassageDialog(text, onHighlight = { highlight(it); choosingPassage = false }, onDismiss = { choosingPassage = false })
    if (lookingUp) PassageDialog(
        text, title = "Look up", hint = "Press and hold a word to select it, then look it up in your dictionary app.", action = "Look up",
        onHighlight = { range -> lookUp(context, text.substring(range.first, range.last + 1)) }, onDismiss = { lookingUp = false },
    )
}

/** The paragraph as selectable text: the reader drags the system selection handles to pick a passage. */
@Composable
private fun PassageDialog(
    text: String,
    onHighlight: (IntRange) -> Unit,
    onDismiss: () -> Unit,
    title: String = "Highlight a passage",
    hint: String = "Press and hold a word, then drag the handles to cover the passage.",
    action: String = "Highlight",
) {
    var value by remember { mutableStateOf(TextFieldValue(text)) }
    val selection = value.selection
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(hint, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(value, { value = it.copy(text = text) }, readOnly = true, maxLines = 12, modifier = Modifier.fillMaxWidth())
            }
        },
        confirmButton = { TextButton(onClick = { onHighlight(selection.min until selection.max) }, enabled = !selection.collapsed) { Text(action) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/**
 * Hands [word] to a dictionary or translate app through the system "process text" action, or a
 * web search when no such app is installed.
 */
internal fun lookUp(context: Context, word: String) {
    val process = Intent(Intent.ACTION_PROCESS_TEXT).setType("text/plain")
        .putExtra(Intent.EXTRA_PROCESS_TEXT, word.trim()).putExtra(Intent.EXTRA_PROCESS_TEXT_READONLY, true)
    val intent = if (context.packageManager.queryIntentActivities(process, 0).isNotEmpty()) Intent.createChooser(process, "Look up \"${word.trim()}\"")
    else Intent(Intent.ACTION_WEB_SEARCH).putExtra(SearchManager.QUERY, "define ${word.trim()}")
    runCatching { context.startActivity(intent) }
}

/** The reading column: capped width, centred, with the reader's chosen margin. */
@Composable
private fun Measure(settings: ReaderSettings, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Box(modifier.fillMaxWidth(), contentAlignment = Alignment.TopCenter) {
        Column(Modifier.widthIn(max = 680.dp).fillMaxWidth().padding(horizontal = settings.horizontalMargin.dp)) { content() }
    }
}

@Composable
private fun BlockView(book: EpubBook, chapterHref: String, block: Block, body: TextStyle, gap: androidx.compose.ui.unit.Dp, onLinkClick: (String) -> Unit, marks: List<IntRange> = emptyList()) {
    when (block) {
        is Block.Heading -> Text(
            linkedText(block.text, block.links, onLinkClick, marks),
            Modifier.padding(top = gap, bottom = gap / 2).semantics { heading() },
            style = when (block.level) { 1 -> MaterialTheme.typography.headlineLarge; 2 -> MaterialTheme.typography.headlineMedium; else -> MaterialTheme.typography.titleLarge },
        )
        is Block.Paragraph -> Text(linkedText(block.text, block.links, onLinkClick, marks), Modifier.padding(bottom = gap), style = body)
        is Block.Quote -> Row(Modifier.padding(bottom = gap)) {
            Box(Modifier.width(3.dp).height(24.dp).background(MaterialTheme.colorScheme.secondary))
            Text(linkedText(block.text, block.links, onLinkClick, marks), Modifier.padding(start = 14.dp), style = body.copy(fontStyle = FontStyle.Italic))
        }
        is Block.Preformatted -> Text(linkedText(block.text, emptyList(), onLinkClick, marks), Modifier.padding(bottom = gap), style = body.copy(fontFamily = FontFamily.Monospace, fontSize = body.fontSize * 0.85, lineHeight = body.lineHeight * 0.9))
        is Block.Rule -> HorizontalDivider(Modifier.padding(vertical = gap), color = MaterialTheme.colorScheme.outline)
        is Block.Image -> BookImage(book, chapterHref, block, Modifier.padding(bottom = gap))
    }
}

/** Reading aids that restyle the text itself, set once per chapter for every block. */
private data class TextAids(val emphasis: Boolean = false, val wordSpacing: Float = 0f)

private val LocalTextAids = staticCompositionLocalOf { TextAids() }

/**
 * [text] with each of [links] underlined and tappable, each of [marks] highlighted, and the
 * reader's word emphasis and word spacing applied. Built once per block, not every frame.
 */
@Composable
private fun linkedText(text: String, links: List<LinkSpan>, onLinkClick: (String) -> Unit, marks: List<IntRange> = emptyList()): AnnotatedString {
    val aids = LocalTextAids.current
    if (links.isEmpty() && marks.isEmpty() && aids == TextAids()) return AnnotatedString(text)
    val color = MaterialTheme.colorScheme.secondary
    val tint = color.copy(alpha = 0.22f)
    val click by rememberUpdatedState(onLinkClick)
    return remember(text, links, marks, aids, color) { buildAnnotatedString {
        append(text)
        if (aids.emphasis) emphasisRanges(text).forEach { addStyle(SpanStyle(fontWeight = FontWeight.Bold), it.first, it.last + 1) }
        // Letter spacing on just the spaces widens the gaps between words.
        if (aids.wordSpacing > 0f) text.forEachIndexed { i, c -> if (c == ' ') addStyle(SpanStyle(letterSpacing = aids.wordSpacing.em), i, i + 1) }
        marks.forEach { if (!it.isEmpty()) addStyle(SpanStyle(background = tint), it.first, it.last + 1) }
        links.forEach { link ->
            if (link.start !in 0..text.length || link.end !in link.start..text.length) return@forEach
            addStyle(SpanStyle(color = color, textDecoration = TextDecoration.Underline), link.start, link.end)
            addLink(LinkAnnotation.Clickable(link.href) { click(link.href) }, link.start, link.end)
        }
    } }
}

/**
 * The opening letters of each word to embolden: one letter of a short word, about 40% of a
 * longer one. Words are runs of letters or digits, so punctuation is never bolded.
 */
internal fun emphasisRanges(text: String): List<IntRange> {
    val ranges = ArrayList<IntRange>()
    var start = -1
    for (i in 0..text.length) {
        val inWord = i < text.length && text[i].isLetterOrDigit()
        if (inWord && start < 0) start = i
        if (!inWord && start >= 0) {
            val length = i - start
            val bold = if (length <= 3) 1 else kotlin.math.ceil(length * 0.4).toInt()
            ranges += start until start + bold
            start = -1
        }
    }
    return ranges
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

/** How the server's placeholder for an unfetched web novel chapter begins. */
private const val WEBNOVEL_PLACEHOLDER = "This chapter is still being fetched."
