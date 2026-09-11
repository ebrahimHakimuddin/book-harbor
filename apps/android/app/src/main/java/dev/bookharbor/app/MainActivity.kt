package dev.bookharbor.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.platform.LocalContext
import dev.bookharbor.app.library.AppController
import dev.bookharbor.app.library.LibraryScreen
import dev.bookharbor.app.library.LibraryUiState
import dev.bookharbor.app.reader.EpubReaderScreen
import dev.bookharbor.app.reader.PdfReaderScreen
import dev.bookharbor.app.reader.ReaderSettingsStore
import dev.bookharbor.app.reader.ReaderTheme
import dev.bookharbor.app.ui.theme.BookHarborTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        enableEdgeToEdge()
        setContent { BookHarborApp() }
    }
}

@Composable
fun BookHarborApp() {
    val context = LocalContext.current
    val graph = remember { AppGraph.get(context) }
    val scope = rememberCoroutineScope()
    val controller = remember { AppController(graph, scope) }
    val settings = remember { ReaderSettingsStore(graph.prefs) }

    LaunchedEffect(Unit) { if (controller.ui == LibraryUiState.Loading) controller.load() }

    val opened = controller.opened
    if (opened != null) {
        // The reader applies its own theme; everything else follows the system.
        BackHandler(onBack = controller::closeReader)
        val common = Triple(opened.book.id, opened.edition.id, opened.book.title)
        if (opened.epub != null) {
            EpubReaderScreen(opened.epub, common.first, common.second, common.third, opened.position, graph.recorder, settings, controller::closeReader)
        } else {
            PdfReaderScreen(opened.file, common.first, common.second, common.third, opened.position, graph.recorder, settings, controller::closeReader)
        }
    } else {
        BookHarborTheme(readerTheme = ReaderTheme.System) { LibraryScreen(controller) }
    }
}
