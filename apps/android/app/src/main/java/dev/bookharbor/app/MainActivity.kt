package dev.bookharbor.app

import android.content.Intent
import android.net.ConnectivityManager
import android.net.Network
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import kotlinx.coroutines.launch
import dev.bookharbor.app.library.AppController
import dev.bookharbor.app.library.LibraryScreen
import dev.bookharbor.app.library.LibraryUiState
import dev.bookharbor.app.reader.EpubReaderScreen
import dev.bookharbor.app.reader.PdfReaderScreen
import dev.bookharbor.app.reader.ReaderSettingsStore
import dev.bookharbor.app.reader.ReaderTheme
import dev.bookharbor.app.ui.theme.BookHarborTheme
import dev.bookharbor.app.widget.ContinueReadingWidget

class MainActivity : ComponentActivity() {
    /** A book the home-screen widget asked to open, until it has been opened. */
    private val openRequest = mutableStateOf<String?>(null)

    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        enableEdgeToEdge()
        if (state == null) openRequest.value = intent.getStringExtra(ContinueReadingWidget.EXTRA_BOOK_ID)
        setContent { BookHarborApp(openRequest) }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        intent.getStringExtra(ContinueReadingWidget.EXTRA_BOOK_ID)?.let { openRequest.value = it }
    }
}

@Composable
fun BookHarborApp(openRequest: MutableState<String?> = remember { mutableStateOf(null) }) {
    val context = LocalContext.current
    val graph = remember { AppGraph.get(context) }
    val scope = rememberCoroutineScope()
    val controller = remember { AppController(graph, scope) }
    val settings = remember { ReaderSettingsStore(graph.prefs) }

    LaunchedEffect(Unit) { if (controller.ui == LibraryUiState.Loading) controller.load() }
    // The saved library appears almost at once (offline first), so the widget's book opens
    // without waiting for the server.
    LaunchedEffect(openRequest.value, controller.ui) {
        val bookId = openRequest.value ?: return@LaunchedEffect
        val catalog = controller.ui as? LibraryUiState.Catalog ?: return@LaunchedEffect
        openRequest.value = null
        val book = catalog.books.firstOrNull { it.id == bookId } ?: return@LaunchedEffect
        if (controller.opened?.book?.id == bookId) return@LaunchedEffect
        if (controller.opened != null) controller.closeReader()
        controller.open(book)
    }
    // When the network comes back while the app is open, refresh an offline library (which also
    // syncs progress) instead of waiting for the reader to pull to refresh.
    DisposableEffect(Unit) {
        val connectivity = context.getSystemService(ConnectivityManager::class.java)
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                scope.launch { if ((controller.ui as? LibraryUiState.Catalog)?.offline == true) controller.refresh() }
            }
        }
        runCatching { connectivity?.registerDefaultNetworkCallback(callback) }
        onDispose { runCatching { connectivity?.unregisterNetworkCallback(callback) } }
    }

    val opened = controller.opened
    if (opened != null) {
        // The reader applies its own theme; everything else follows the system. Each reader
        // screen owns its own back handling so it can flush pending progress before closing.
        val common = Triple(opened.book.id, opened.edition.id, opened.book.title)
        if (opened.epub != null) {
            EpubReaderScreen(opened.epub, common.first, common.second, common.third, opened.position, graph.recorder, settings, graph.annotations, graph.readingStats, controller::closeReader)
        } else {
            PdfReaderScreen(opened.file, common.first, common.second, common.third, opened.position, graph.recorder, settings, graph.annotations, graph.readingStats, controller::closeReader)
        }
    } else {
        BookHarborTheme(readerTheme = ReaderTheme.System) {
            Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) { LibraryScreen(controller) }
        }
    }
}
