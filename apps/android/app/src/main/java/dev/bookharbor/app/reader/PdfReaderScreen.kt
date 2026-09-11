package dev.bookharbor.app.reader

import android.graphics.Bitmap
import android.graphics.Color
import android.graphics.pdf.PdfRenderer
import android.os.ParcelFileDescriptor
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.ColorFilter
import androidx.compose.ui.graphics.ColorMatrix
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import dev.bookharbor.app.sync.LocalPosition
import dev.bookharbor.app.sync.Locator
import dev.bookharbor.app.sync.ProgressRecorder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.withContext
import java.io.Closeable
import java.io.File

/**
 * A PDF opened with the platform renderer. PdfRenderer allows one open page at a time, so every
 * page access is serialized; rendered bitmaps are kept in a small byte-bounded cache.
 */
class PdfBook(file: File) : Closeable {
    private val descriptor = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
    private val renderer = PdfRenderer(descriptor)
    private val cache = object : LruCache<String, Bitmap>(48 * 1024 * 1024) {
        override fun sizeOf(key: String, value: Bitmap) = value.byteCount
    }

    val pageCount: Int = renderer.pageCount

    /** Height ÷ width of each page, read once so the list keeps a stable layout while pages render. */
    @Synchronized
    fun aspectRatios(): List<Float> = List(pageCount) { index ->
        renderer.openPage(index).use { page -> (page.height.toFloat() / page.width.toFloat()).coerceIn(0.2f, 5f) }
    }

    @Synchronized
    fun render(index: Int, widthPx: Int): Bitmap {
        val key = "$index@$widthPx"
        cache.get(key)?.let { return it }
        return renderer.openPage(index).use { page ->
            val height = (widthPx * page.height.toFloat() / page.width).toInt().coerceAtLeast(1)
            Bitmap.createBitmap(widthPx, height, Bitmap.Config.ARGB_8888).also {
                it.eraseColor(Color.WHITE)
                page.render(it, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                cache.put(key, it)
            }
        }
    }

    @Synchronized
    override fun close() {
        cache.evictAll()
        renderer.close()
        descriptor.close()
    }
}

private val InvertLuminance = ColorFilter.colorMatrix(
    ColorMatrix(floatArrayOf(-1f, 0f, 0f, 0f, 255f, 0f, -1f, 0f, 0f, 255f, 0f, 0f, -1f, 0f, 255f, 0f, 0f, 0f, 1f, 0f)),
)

@OptIn(FlowPreview::class)
@Composable
fun PdfReaderScreen(
    file: File,
    bookId: String,
    editionId: String,
    bookTitle: String,
    restore: LocalPosition?,
    recorder: ProgressRecorder,
    settingsStore: ReaderSettingsStore,
    onClose: () -> Unit,
) {
    val opened by produceState<Result<PdfBook>?>(null, file) { value = withContext(Dispatchers.IO) { runCatching { PdfBook(file) } } }
    val book = opened?.getOrNull()
    DisposableEffect(book) { onDispose { book?.close() } }

    if (opened == null) {
        Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        return
    }
    if (book == null) {
        val reason = opened?.exceptionOrNull()
        Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).padding(32.dp), verticalArrangement = Arrangement.Center) {
            Text("This PDF can't be opened", style = MaterialTheme.typography.headlineMedium)
            Text(if (reason is SecurityException) "It is password protected." else "The file may be damaged. Remove the download and fetch it again.", Modifier.padding(vertical = 12.dp))
            Button(onClick = onClose) { Text("Back to library") }
        }
        return
    }
    PdfPages(book, bookId, editionId, bookTitle, restore, recorder, settingsStore, onClose)
}

@OptIn(FlowPreview::class)
@Composable
private fun PdfPages(
    book: PdfBook,
    bookId: String,
    editionId: String,
    bookTitle: String,
    restore: LocalPosition?,
    recorder: ProgressRecorder,
    settingsStore: ReaderSettingsStore,
    onClose: () -> Unit,
) {
    val count = book.pageCount
    val startPage = remember { restore?.locator?.takeIf { it.kind == Locator.PDF }?.page?.coerceIn(1, count) ?: 1 }
    var state by remember {
        mutableStateOf(
            ReaderState(
                bookTitle = bookTitle,
                chapters = List(count) { ReaderChapter("page-$it", "Page ${it + 1}") },
                chapterIndex = startPage - 1,
                settings = settingsStore.load(),
            ),
        )
    }
    LaunchedEffect(state.settings) { settingsStore.save(state.settings) }
    val listState = rememberLazyListState()
    val ratios by produceState<List<Float>?>(null, book) { value = withContext(Dispatchers.IO) { runCatching { book.aspectRatios() }.getOrNull() } }
    var ready by remember { mutableStateOf(false) }
    var latest by remember { mutableStateOf(startPage) }

    fun persist(page: Int) {
        recorder.record(bookId, editionId, Locator.pdf(page), page.toDouble() / count)
    }

    LaunchedEffect(ratios) {
        if (ratios == null) return@LaunchedEffect
        listState.scrollToItem(startPage - 1)
        ready = true
    }
    LaunchedEffect(ready) {
        if (!ready) return@LaunchedEffect
        snapshotFlow { firstVisiblePage(listState) }
            .distinctUntilChanged()
            .onEach { latest = it; state = state.copy(chapterIndex = it - 1) }
            .debounce(400)
            .collect { page -> withContext(Dispatchers.IO) { persist(page) } }
    }
    LifecycleEventEffect(Lifecycle.Event.ON_STOP) { persist(latest) }
    DisposableEffect(Unit) { onDispose { persist(latest) } }

    // "Contents" for a PDF is a page list; choosing a page scrolls there.
    var jumpTarget by remember { mutableStateOf<Int?>(null) }
    LaunchedEffect(jumpTarget) {
        jumpTarget?.let { listState.scrollToItem(it); jumpTarget = null }
    }

    ReaderScaffold(
        state = state,
        onAction = { action ->
            state = state.reduce(action)
            if (action is ReaderAction.SelectChapter && action.index in 0 until count) {
                latest = action.index + 1
                jumpTarget = action.index
            }
        },
        onClose = onClose,
        progressLabel = "Page ${state.chapterIndex + 1} of $count",
        fixedLayout = true,
        contentsLabel = "Pages",
    ) { padding -> PageList(book, ratios, listState, padding, state.settings.theme) }
}

private fun firstVisiblePage(state: LazyListState): Int {
    val item = state.layoutInfo.visibleItemsInfo.firstOrNull { it.offset + it.size > 96 } ?: return 1
    return item.index + 1
}

@Composable
private fun PageList(book: PdfBook, ratios: List<Float>?, listState: LazyListState, padding: PaddingValues, theme: ReaderTheme) {
    if (ratios == null) {
        Box(Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        return
    }
    val dark = theme == ReaderTheme.Dark || theme == ReaderTheme.Black
    BoxWithConstraints(Modifier.fillMaxSize().padding(padding)) {
        val widthPx = with(LocalDensity.current) { maxWidth.roundToPx() }.coerceIn(200, 1600)
        LazyColumn(state = listState, modifier = Modifier.fillMaxSize(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            itemsIndexed(ratios, key = { index, _ -> index }) { index, ratio ->
                val bitmap by produceState<Bitmap?>(null, index, widthPx) { value = withContext(Dispatchers.IO) { runCatching { book.render(index, widthPx) }.getOrNull() } }
                val page = bitmap
                Box(Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surfaceVariant)) {
                    if (page != null) {
                        Image(page.asImageBitmap(), contentDescription = "Page ${index + 1}", modifier = Modifier.fillMaxWidth(), contentScale = ContentScale.FillWidth, colorFilter = if (dark) InvertLuminance else null)
                    } else {
                        Box(Modifier.fillMaxWidth().aspectRatio(1f / ratio), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
                    }
                }
            }
        }
    }
}
