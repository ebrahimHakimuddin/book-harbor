package dev.bookharbor.app.library

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import dev.bookharbor.app.AppGraph
import dev.bookharbor.app.reader.epub.EpubBook
import dev.bookharbor.app.sync.LocalPosition
import dev.bookharbor.app.sync.SyncScheduler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import java.io.File

sealed interface LibraryUiState {
    data object Setup : LibraryUiState
    data class SignIn(val instance: InstanceInfo? = null, val serverUrl: String = "") : LibraryUiState
    data object Loading : LibraryUiState
    data class Catalog(
        val instanceName: String,
        val books: List<Book>,
        val downloads: Map<String, DownloadStatus> = emptyMap(),
        val errors: Map<String, String> = emptyMap(),
        val progress: Map<String, Double> = emptyMap(),
        /** True when showing the last saved catalog because the server could not be reached. */
        val offline: Boolean = false,
    ) : LibraryUiState
    data class Error(val message: String, val retry: LibraryUiState) : LibraryUiState
}

data class SyncUiState(val pending: Int = 0, val rejected: Int = 0, val running: Boolean = false, val error: String? = null, val lastSyncMillis: Long = 0)

/** A book that is open in the reader. [epub] is already parsed, so open errors surface before the screen changes. */
class OpenedBook(val book: Book, val edition: Edition, val file: File, val position: LocalPosition?, val epub: EpubBook?)

/** Everything the screens read and every action they can take. Work runs on IO; state is Compose state. */
class AppController(private val graph: AppGraph, private val scope: CoroutineScope) {
    var ui by mutableStateOf<LibraryUiState>(if (graph.session.serverUrl.isBlank()) LibraryUiState.Setup else LibraryUiState.Loading)
        private set
    var opened by mutableStateOf<OpenedBook?>(null)
        private set
    var sync by mutableStateOf(SyncUiState())
        private set
    /** Set when signing out would discard reading updates that have not reached the server. */
    var unsyncedOnSignOut by mutableStateOf<Int?>(null)
        private set
    var notice by mutableStateOf<String?>(null)
    val covers = CoverLoader(graph.api, graph.cacheDir)
    val serverUrl: String get() = graph.session.serverUrl
    val displayName: String get() = graph.session.displayName

    private val cache = CatalogCache(graph.prefs)

    fun load() {
        ui = LibraryUiState.Loading
        scope.launch(Dispatchers.IO) {
            try {
                val instance = graph.library.instance(graph.session.serverUrl)
                ui = when {
                    instance.setupRequired -> LibraryUiState.Error("This BookHarbor server still needs to be set up by an administrator.", LibraryUiState.Setup)
                    graph.session.tokens == null -> LibraryUiState.SignIn(instance, graph.session.serverUrl)
                    else -> {
                        val books = graph.library.books()
                        cache.save(instance.name, books)
                        SyncScheduler.schedule(graph.context)
                        catalog(instance.name, books, offline = false)
                    }
                }
            } catch (error: Exception) {
                ui = when {
                    error is HttpError && error.status == 401 -> { graph.session.tokens = null; LibraryUiState.SignIn(null, graph.session.serverUrl) }
                    graph.session.tokens != null && cache.load() != null -> cache.load()!!.let { (name, books) -> catalog(name, books, offline = true) }
                    else -> LibraryUiState.Error(error.message?.takeIf { it.isNotBlank() } ?: "Unable to connect", if (graph.session.serverUrl.isBlank()) LibraryUiState.Setup else LibraryUiState.Loading)
                }
            }
            refreshSync()
        }
    }

    /** Leaves an error screen for the state it came from. */
    fun dismissError(to: LibraryUiState) { ui = to }

    fun connect(url: String) { graph.session.serverUrl = url; load() }
    fun changeServer() { ui = LibraryUiState.Setup }

    fun signIn(email: String, password: String) {
        ui = LibraryUiState.Loading
        scope.launch(Dispatchers.IO) {
            try {
                graph.library.signIn(graph.session.serverUrl, email, password)
                load()
            } catch (error: Exception) {
                val message = if (error is HttpError && error.status == 401) "That email or password isn't right." else (error.message ?: "Sign in failed")
                ui = LibraryUiState.Error(message, LibraryUiState.SignIn(serverUrl = graph.session.serverUrl))
            }
        }
    }

    /**
     * Signs out. Unsent reading updates are synced first; if some remain the caller is told via
     * [unsyncedOnSignOut] and must confirm with [force] = true, because they will be discarded.
     */
    fun signOut(force: Boolean = false) {
        scope.launch(Dispatchers.IO) {
            if (!force && graph.progress.pendingCount() > 0) {
                runCatching { graph.syncEngine.runOnce() }
                val left = graph.progress.pendingCount()
                if (left > 0) { unsyncedOnSignOut = left; return@launch }
            }
            unsyncedOnSignOut = null
            graph.library.signOut()
            graph.session.tokens = null
            graph.session.displayName = ""
            graph.progress.clear() // never send this account's unsent events as someone else
            cache.clear()
            opened = null
            ui = LibraryUiState.SignIn(serverUrl = graph.session.serverUrl)
            refreshSync()
        }
    }

    fun dismissSignOutWarning() { unsyncedOnSignOut = null }

    private fun catalog(name: String, books: List<Book>, offline: Boolean): LibraryUiState.Catalog {
        val available = graph.downloads.all().map { it.editionId }.toSet()
        return LibraryUiState.Catalog(
            instanceName = name,
            books = books,
            downloads = books.flatMap { it.editions }.associate { it.id to if (it.id in available) DownloadStatus.AVAILABLE else DownloadStatus.NOT_DOWNLOADED },
            progress = graph.progress.positions().associate { it.bookId to it.percentage },
            offline = offline,
        )
    }

    private fun updateCatalog(change: (LibraryUiState.Catalog) -> LibraryUiState.Catalog) {
        val current = ui
        if (current is LibraryUiState.Catalog) ui = change(current)
    }

    fun download(edition: Edition, thenOpen: Book? = null) {
        updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.DOWNLOADING), errors = it.errors - edition.id) }
        scope.launch(Dispatchers.IO) {
            try {
                graph.downloader.download(edition)
                updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.AVAILABLE)) }
                if (thenOpen != null) open(thenOpen)
            } catch (error: Exception) {
                updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.FAILED), errors = it.errors + (edition.id to (error.message ?: "Download failed"))) }
            }
        }
    }

    fun removeDownload(edition: Edition) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.downloader.remove(edition.id)
                updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.NOT_DOWNLOADED), errors = it.errors - edition.id) }
            } catch (error: Exception) {
                updateCatalog { it.copy(errors = it.errors + (edition.id to (error.message ?: "Could not remove the download"))) }
            }
        }
    }

    /** Opens a book offline if any edition is downloaded; otherwise downloads the best edition and then opens it. */
    fun open(book: Book) {
        val catalog = ui as? LibraryUiState.Catalog ?: return
        val edition = preferredEdition(book) { catalog.downloads[it.id] == DownloadStatus.AVAILABLE } ?: return
        if (catalog.downloads[edition.id] != DownloadStatus.AVAILABLE) { download(edition, thenOpen = book); return }
        scope.launch(Dispatchers.IO) {
            val local = graph.downloads.get(edition.id) // re-verifies the checksum before trusting the file
            if (local == null) {
                updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.FAILED), errors = it.errors + (edition.id to "This download was damaged. Download it again.")) }
                return@launch
            }
            val file = File(local.path)
            try {
                val epub = if (edition.format == "epub") EpubBook.open(file) else null
                opened = OpenedBook(book, edition, file, graph.progress.position(book.id), epub)
            } catch (error: Exception) {
                updateCatalog { it.copy(errors = it.errors + (edition.id to (error.message ?: "This file could not be opened"))) }
            }
        }
    }

    fun closeReader() {
        opened?.epub?.close()
        opened = null
        updateCatalog { it.copy(progress = graph.progress.positions().associate { p -> p.bookId to p.percentage }) }
        scope.launch(Dispatchers.IO) { refreshSync() }
    }

    fun syncNow() {
        if (sync.running) return
        sync = sync.copy(running = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                graph.syncEngine.runOnce()
                sync = sync.copy(lastSyncMillis = System.currentTimeMillis())
            } catch (error: Exception) {
                sync = sync.copy(error = "Couldn't reach the server. Your progress is saved here and will sync when you're back online.")
            }
            refreshSync()
            updateCatalog { it.copy(progress = graph.progress.positions().associate { p -> p.bookId to p.percentage }) }
        }
    }

    private fun refreshSync() {
        sync = sync.copy(pending = graph.progress.pendingCount(), rejected = graph.progress.rejectedCount(), running = false)
    }
}
