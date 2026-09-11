package dev.bookharbor.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.*
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.tooling.preview.Preview
import dev.bookharbor.app.library.*
import dev.bookharbor.app.reader.ReaderScreen
import dev.bookharbor.app.reader.ReaderState
import dev.bookharbor.app.reader.ReaderTheme
import dev.bookharbor.app.ui.theme.BookHarborTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() { override fun onCreate(state: Bundle?) { super.onCreate(state); enableEdgeToEdge(); setContent { BookHarborApp() } } }

@Composable fun BookHarborApp() {
    val context = LocalContext.current
    val graph = remember { AppGraph.get(context) }
    val store = graph.session
    val downloadStore = graph.downloads
    val client = graph.library
    val downloader = graph.downloader
    val scope = rememberCoroutineScope()
    var uiState by remember { mutableStateOf<LibraryUiState>(if (store.serverUrl.isBlank()) LibraryUiState.Setup else LibraryUiState.Loading) }
    fun updateCatalog(update: (LibraryUiState.Catalog) -> LibraryUiState.Catalog) { val current = uiState; if (current is LibraryUiState.Catalog) uiState = update(current) }
    fun load() { uiState = LibraryUiState.Loading; scope.launch(Dispatchers.IO) { try { val instance = client.instance(store.serverUrl); uiState = when { instance.setupRequired -> LibraryUiState.Error("This BookHarbor server still needs to be set up by an administrator.", LibraryUiState.Setup); store.tokens == null -> LibraryUiState.SignIn(instance, store.serverUrl); else -> { val books = client.books(); LibraryUiState.Catalog(instance, books, books.flatMap { it.editions }.associate { it.id to if (downloadStore.get(it.id) != null) DownloadStatus.AVAILABLE else DownloadStatus.NOT_DOWNLOADED }) } } } catch (e: Exception) { uiState = LibraryUiState.Error(e.message ?: "Unable to connect", LibraryUiState.Setup) } } }
    fun download(edition: Edition) { val current = uiState as? LibraryUiState.Catalog ?: return; uiState = current.copy(downloads = current.downloads + (edition.id to DownloadStatus.DOWNLOADING), errors = current.errors - edition.id); scope.launch(Dispatchers.IO) { try { downloader.download(edition); updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.AVAILABLE)) } } catch (error: Exception) { updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.FAILED), errors = it.errors + (edition.id to (error.message ?: "Download failed"))) } } } }
    fun remove(edition: Edition) { val current = uiState as? LibraryUiState.Catalog ?: return; scope.launch(Dispatchers.IO) { try { downloader.remove(edition.id); updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.NOT_DOWNLOADED), errors = it.errors - edition.id) } } catch (error: Exception) { updateCatalog { it.copy(errors = it.errors + (edition.id to (error.message ?: "Could not remove download"))) } } } }
    LaunchedEffect(Unit) { if (uiState == LibraryUiState.Loading) load() }
    BookHarborTheme(readerTheme = ReaderTheme.System) { LibraryScreen(uiState, onServer = { store.serverUrl = it; load() }, onSignIn = { email, password -> uiState = LibraryUiState.Loading; scope.launch(Dispatchers.IO) { try { client.signIn(store.serverUrl, email, password); load() } catch (e: Exception) { uiState = LibraryUiState.Error(e.message ?: "Sign in failed", LibraryUiState.SignIn(serverUrl = store.serverUrl)) } } }, onRetry = { load() }, onSignOut = { scope.launch(Dispatchers.IO) { client.signOut(); store.tokens = null; uiState = LibraryUiState.SignIn(serverUrl = store.serverUrl) } }, onDownload = ::download, onRemove = ::remove) }
}

@Preview(showBackground = true, widthDp = 393, heightDp = 852)
@Composable private fun BookHarborPreview() { BookHarborTheme(readerTheme = ReaderTheme.System) { ReaderScreen(ReaderState.preview(), {}, {}) } }
