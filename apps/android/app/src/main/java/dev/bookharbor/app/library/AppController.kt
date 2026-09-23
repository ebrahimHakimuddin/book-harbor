package dev.bookharbor.app.library

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import dev.bookharbor.app.AppGraph
import dev.bookharbor.app.notify.Notifications
import dev.bookharbor.app.reader.chaptersLeft
import dev.bookharbor.app.reader.epub.EpubBook
import dev.bookharbor.app.widget.ContinueReadingWidget
import dev.bookharbor.app.reader.epub.EpubPosition
import dev.bookharbor.app.sync.LocalPosition
import dev.bookharbor.app.sync.Locator
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

sealed interface LibraryUiState {
    data object Setup : LibraryUiState
    data class SignIn(val instance: InstanceInfo? = null, val serverUrl: String = "", val email: String = "") : LibraryUiState
    data object Loading : LibraryUiState
    data class Catalog(
        val instanceName: String,
        val books: List<Book>,
        val downloads: Map<String, DownloadStatus> = emptyMap(),
        val errors: Map<String, String> = emptyMap(),
        /** 0..1 for each edition that is downloading and knows its size. */
        val downloadProgress: Map<String, Float> = emptyMap(),
        val progress: Map<String, Double> = emptyMap(),
        /** Unread chapters per book, for books whose chapter count is known (EPUBs opened here). */
        val chaptersLeft: Map<String, Int> = emptyMap(),
        /** RFC 3339 UTC instant of each book's most recent local reading event, for the History tab. */
        val lastReadAt: Map<String, String> = emptyMap(),
        /** True when showing the last saved catalog because the server could not be reached. */
        val offline: Boolean = false,
    ) : LibraryUiState
    data class Error(val message: String, val retry: LibraryUiState) : LibraryUiState
}

data class SyncUiState(val pending: Int = 0, val rejected: Int = 0, val running: Boolean = false, val error: String? = null, val lastSyncMillis: Long = 0)

/** The sign-in screen's "forgot password" flow: ask for a code, then enter it with a new password. */
data class PasswordResetUiState(val codeSent: Boolean = false, val working: Boolean = false, val error: String? = null, val done: Boolean = false)

data class ProfileUiState(val saving: Boolean = false, val error: String? = null, val passwordChanged: Boolean = false)

data class FriendsUiState(
    val friends: List<Friend> = emptyList(),
    val incoming: List<FriendRequest> = emptyList(),
    val outgoing: List<FriendRequest> = emptyList(),
    val settings: SocialSettings = SocialSettings(activityVisible = false, goalYear = 0, goalBooks = 0),
    val loading: Boolean = false,
    val error: String? = null,
)

data class ListsUiState(
    val lists: List<BookList> = emptyList(),
    val openList: BookList? = null,
    val booksInOpenList: List<Book> = emptyList(),
    val loading: Boolean = false,
    val error: String? = null,
)

data class BookRequestsUiState(
    val mine: List<BookRequest> = emptyList(),
    val searchResults: List<MetadataCandidate> = emptyList(),
    val searching: Boolean = false,
    val loading: Boolean = false,
    val error: String? = null,
    val notice: String? = null,
)

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
    var friendsUi by mutableStateOf(FriendsUiState())
        private set
    var profileUi by mutableStateOf(ProfileUiState())
        private set
    /** The friend page on screen: null while loading, or after an error ([friendsUi] has it). */
    var friendProfile by mutableStateOf<FriendProfile?>(null)
        private set
    var bookRequestsUi by mutableStateOf(BookRequestsUiState())
        private set
    var listsUi by mutableStateOf(ListsUiState())
        private set
    /** Set when signing out would discard reading updates that have not reached the server. */
    var unsyncedOnSignOut by mutableStateOf<Int?>(null)
        private set
    var notice by mutableStateOf<String?>(null)
    /** The book whose details page is open, over whichever screen opened it. */
    var detailsBook by mutableStateOf<Book?>(null)
        private set

    fun showDetails(book: Book) { detailsBook = book }
    fun closeDetails() { detailsBook = null }
    var passwordResetUi by mutableStateOf(PasswordResetUiState())
        private set
    val covers = CoverLoader(graph.api, graph.cacheDir)
    val serverUrl: String get() = graph.session.serverUrl
    val displayName: String get() = graph.session.displayName

    private val cache = CatalogCache(graph.prefs)
    /** The server's self-description, kept so the sign-in form still knows what it offers after a failed attempt. */
    private var lastInstance: InstanceInfo? = null

    /** True while a pull-to-refresh reload is running; the catalog stays on screen meanwhile. */
    var refreshing by mutableStateOf(false)
        private set

    fun refresh() {
        if (refreshing) return
        refreshing = true
        load(quiet = true)
    }

    /**
     * Offline first: a signed-in reader gets their saved library immediately, and the network only
     * refreshes it. A slow or unreachable server never stands between them and their books.
     */
    fun load(quiet: Boolean = false) {
        if (!quiet && ui !is LibraryUiState.Catalog) ui = LibraryUiState.Loading
        scope.launch(Dispatchers.IO) {
            if (ui !is LibraryUiState.Catalog && graph.session.tokens != null) {
                cache.load()?.let { (name, books) ->
                    // The saved copy is up; checking it against the server happens quietly. The
                    // pull-to-refresh spinner is only for refreshes the reader asked for -- as an
                    // automatic check it flashed for a frame on every launch.
                    ui = catalog(name, books, offline = false)
                }
            }
            try {
                val instance = graph.library.instance(graph.session.serverUrl)
                lastInstance = instance
                ui = when {
                    instance.setupRequired -> LibraryUiState.Error("This BookHarbor server still needs to be set up by an administrator.", LibraryUiState.Setup)
                    graph.session.tokens == null -> LibraryUiState.SignIn(instance, graph.session.serverUrl)
                    else -> {
                        val books = graph.library.books()
                        cache.save(instance.name, books)
                        ContinueReadingWidget.refresh(graph.context)
                        catalog(instance.name, books, offline = false)
                    }
                }
            } catch (error: Exception) {
                ui = when {
                    error is HttpError && error.status == 401 -> { graph.session.tokens = null; LibraryUiState.SignIn(lastInstance, graph.session.serverUrl) }
                    graph.session.tokens != null && cache.load() != null -> cache.load()!!.let { (name, books) -> catalog(name, books, offline = true) }
                    else -> LibraryUiState.Error(error.message?.takeIf { it.isNotBlank() } ?: "Unable to connect", if (graph.session.serverUrl.isBlank()) LibraryUiState.Setup else LibraryUiState.Loading)
                }
            }
            refreshing = false
            refreshSync()
            // Back online: push queued progress and pull other devices' positions into the library now.
            if ((ui as? LibraryUiState.Catalog)?.offline == false) syncNow()
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
                ui = LibraryUiState.Error(message, LibraryUiState.SignIn(lastInstance, graph.session.serverUrl, email))
            }
        }
    }

/** Where signing out lands: back at this server's sign-in form, or at server setup to point at a different one. */
    enum class SignOutTarget { SIGN_IN, SETUP }

    private var signOutTarget = SignOutTarget.SIGN_IN

    /**
     * Signs out. Unsent reading updates are synced first; if some remain the caller is told via
     * [unsyncedOnSignOut] and must confirm with [force] = true, because they will be discarded.
     */
    fun signOut(force: Boolean = false, target: SignOutTarget = SignOutTarget.SIGN_IN) {
        signOutTarget = target
        scope.launch(Dispatchers.IO) {
            if (!force && unsynced() > 0) {
                runCatching { graph.syncEngine.runOnce() }
                runCatching { graph.annotationSync.runOnce() }
                val left = unsynced()
                if (left > 0) { unsyncedOnSignOut = left; return@launch }
            }
            unsyncedOnSignOut = null
            graph.library.signOut()
            graph.session.tokens = null
            graph.session.displayName = ""
            graph.progress.clear() // never send this account's unsent events as someone else
            graph.annotations.clear() // they belong to this account, and are on its server
            Notifications.forget(graph.prefs) // the next account's library isn't "new"
            ContinueReadingWidget.refresh(graph.context)
            cache.clear()
            opened = null
            ui = when (signOutTarget) {
                SignOutTarget.SIGN_IN -> LibraryUiState.SignIn(lastInstance, graph.session.serverUrl)
                SignOutTarget.SETUP -> { graph.session.serverUrl = ""; LibraryUiState.Setup }
            }
            refreshSync()
        }
    }

    private fun unsynced() = graph.progress.pendingCount() + graph.annotations.pendingCount()

    /** Signs out and lands on server setup, so the reader can point the app at a different server. */
    fun switchServer(force: Boolean = false) = signOut(force, SignOutTarget.SETUP)

    /** Confirms the sign-out [unsyncedOnSignOut] warned about, keeping whichever target the original attempt asked for. */
    fun confirmSignOut() = signOut(force = true, target = signOutTarget)

    fun dismissSignOutWarning() { unsyncedOnSignOut = null }

    private fun catalog(name: String, books: List<Book>, offline: Boolean): LibraryUiState.Catalog {
        val available = graph.downloads.all().map { it.editionId }.toSet()
        return withReading(LibraryUiState.Catalog(
            instanceName = name,
            books = books,
            downloads = books.flatMap { it.editions }.associate { it.id to if (it.id in available) DownloadStatus.AVAILABLE else DownloadStatus.NOT_DOWNLOADED },
            offline = offline,
        ))
    }

    /** [catalog] with reading state (progress, last read, chapters left) re-read from this device. */
    private fun withReading(catalog: LibraryUiState.Catalog): LibraryUiState.Catalog {
        val positions = graph.progress.positions().associateBy { it.bookId }
        val left = graph.chapterMarks.counts().mapValues { (bookId, count) ->
            val position = positions[bookId]
            val chapter = position?.locator?.takeIf { it.kind == Locator.EPUB }?.let { EpubPosition.parse(it.value)?.chapterIndex }
            chaptersLeft(count, graph.chapterMarks.get(bookId), chapter, isFinished(position?.percentage))
        }
        return catalog.copy(
            progress = positions.mapValues { it.value.percentage },
            lastReadAt = positions.mapValues { it.value.occurredAt },
            chaptersLeft = left,
        )
    }

    private fun refreshReading() = updateCatalog(::withReading)


    /** Serialized: downloads report progress from several IO threads at once, and none may lose another's change. */
    private val catalogLock = Any()

    private fun updateCatalog(change: (LibraryUiState.Catalog) -> LibraryUiState.Catalog) = synchronized(catalogLock) {
        val current = ui
        if (current is LibraryUiState.Catalog) ui = change(current)
    }

    fun download(edition: Edition, thenOpen: Book? = null) {
        scope.launch {
            downloadNow(edition)
            if (thenOpen != null && (ui as? LibraryUiState.Catalog)?.downloads?.get(edition.id) == DownloadStatus.AVAILABLE) open(thenOpen)
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

    /**
     * Opens a book offline if any edition is downloaded; otherwise downloads the best edition and then opens it.
     * [edition] and [startAt] open a specific edition at a chosen chapter or page instead of the saved place.
     */
    fun open(book: Book, edition: Edition? = null, startAt: Locator? = null) {
        val catalog = ui as? LibraryUiState.Catalog ?: return
        val edition = edition ?: preferredEdition(book) { catalog.downloads[it.id] == DownloadStatus.AVAILABLE } ?: return
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
                epub?.let { graph.chapterMarks.setCount(book.id, it.chapters.size) }
                val position = startAt?.let { LocalPosition(book.id, edition.id, it, 0.0, "", "") } ?: graph.progress.position(book.id)
                opened = OpenedBook(book, edition, file, position, epub)
            } catch (error: Exception) {
                updateCatalog { it.copy(errors = it.errors + (edition.id to (error.message ?: "This file could not be opened"))) }
            }
        }
    }

    /**
     * Chapter titles of a downloaded EPUB, or "Page n" for each page of a PDF, for opening at a
     * chosen place. Null when the edition isn't on the device or can't be read.
     */
    suspend fun tableOfContents(book: Book, edition: Edition): List<String>? = withContext(Dispatchers.IO) {
        val local = graph.downloads.get(edition.id) ?: return@withContext null
        runCatching {
            if (edition.format == "epub") EpubBook.open(File(local.path)).use { epub ->
                graph.chapterMarks.setCount(book.id, epub.chapters.size)
                epub.chapters.mapIndexed { i, c -> c.title.ifBlank { "Chapter ${i + 1}" } }
            }
            else android.os.ParcelFileDescriptor.open(File(local.path), android.os.ParcelFileDescriptor.MODE_READ_ONLY).use { fd ->
                android.graphics.pdf.PdfRenderer(fd).use { pdf -> List(pdf.pageCount) { "Page ${it + 1}" } }
            }
        }.getOrNull()
    }

    /**
     * Marks a book finished (100%) or not started (0%) as an ordinary reading event, so it syncs
     * like any other progress. Finishing keeps the reader's place; unreading goes back to the start.
     */
    fun markRead(book: Book, read: Boolean) {
        val catalog = ui as? LibraryUiState.Catalog ?: return
        val edition = preferredEdition(book) { catalog.downloads[it.id] == DownloadStatus.AVAILABLE } ?: return
        scope.launch(Dispatchers.IO) {
            val current = graph.progress.position(book.id)
            val start = if (edition.format == "epub") Locator.epub(EpubPosition.atChapter(0).toCfi()) else Locator.pdf(1)
            val locator = if (read && current != null && current.editionId == edition.id) current.locator else start
            graph.recorder.record(book.id, current?.editionId?.takeIf { read } ?: edition.id, locator, if (read) 1.0 else 0.0)
            // Unreading a book starts its chapters over too.
            if (!read) { graph.chapterMarks.clear(book.id); chapterMarksVersion++ }
            refreshReading()
            ContinueReadingWidget.refresh(graph.context)
            notice = if (read) "Marked \"${book.title}\" as read" else "Marked \"${book.title}\" as unread"
        }
    }

    /** Bumped on every chapter mark, so screens showing chapter state recompose. */
    var chapterMarksVersion by mutableIntStateOf(0)
        private set

    fun chapterMarks(bookId: String): Map<Int, Boolean> = graph.chapterMarks.get(bookId)

    fun markChapters(book: Book, chapters: Collection<Int>, read: Boolean) {
        graph.chapterMarks.set(book.id, chapters, read)
        chapterMarksVersion++
        refreshReading()
        notice = "Marked ${chapters.size} ${if (chapters.size == 1) "chapter" else "chapters"} as ${if (read) "read" else "unread"}"
    }

    fun markChaptersReadUpTo(book: Book, index: Int) {
        graph.chapterMarks.markReadUpTo(book.id, index)
        chapterMarksVersion++
        refreshReading()
        notice = "Marked chapters 1–${index + 1} as read"
    }

    /** Where this device last left [bookId], if it has been opened. */
    fun savedPosition(bookId: String): LocalPosition? = graph.progress.position(bookId)

    var notificationsEnabled by mutableStateOf(Notifications.enabled(graph.prefs))
        private set

    /** True once: the first time the library opens, to ask for the notification permission. */
    fun firstNotificationPrompt(): Boolean {
        if (graph.prefs.getBoolean("notify.asked", false)) return false
        graph.prefs.edit().putBoolean("notify.asked", true).apply()
        return notificationsEnabled
    }

    fun setNotifications(enabled: Boolean) {
        Notifications.setEnabled(graph.context, graph.prefs, enabled)
        notificationsEnabled = enabled
    }

    /** Bytes used by downloaded books on this device. */
    fun downloadedBytes(): Long = graph.downloads.all().sumOf { File(it.path).length() }

    /** Frees space held by books the reader has finished; they can be downloaded again any time. */
    fun removeFinishedDownloads() {
        val catalog = ui as? LibraryUiState.Catalog ?: return
        val editions = catalog.books.filter { isFinished(catalog.progress[it.id]) }.flatMap { it.editions }.filter { catalog.downloads[it.id] == DownloadStatus.AVAILABLE }
        editions.forEach(::removeDownload)
        notice = if (editions.isEmpty()) "No finished books are downloaded" else "Removed ${editions.size} finished ${if (editions.size == 1) "download" else "downloads"}"
    }

    /** Downloads the preferred edition of every book in [books] that isn't on the device yet. */
    fun downloadAll(books: List<Book>) {
        val catalog = ui as? LibraryUiState.Catalog ?: return
        val missing = books.filter { book -> book.editions.none { catalog.downloads[it.id] == DownloadStatus.AVAILABLE || catalog.downloads[it.id] == DownloadStatus.DOWNLOADING } }
        // One at a time, so a list of twenty books doesn't open twenty connections at once.
        scope.launch {
            missing.forEach { book -> preferredEdition(book) { false }?.let { edition -> downloadNow(edition) } }
        }
        notice = if (missing.isEmpty()) "Everything here is already downloaded" else "Downloading ${missing.size} ${if (missing.size == 1) "book" else "books"}"
    }

    private suspend fun downloadNow(edition: Edition) = withContext(Dispatchers.IO) {
        updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.DOWNLOADING), errors = it.errors - edition.id) }
        try {
            val local = graph.downloader.download(edition) { fraction -> updateCatalog { it.copy(downloadProgress = it.downloadProgress + (edition.id to fraction)) } }
            // Count chapters now, so the book's card can show how many are left before it's opened.
            if (edition.format == "epub") {
                val owner = (ui as? LibraryUiState.Catalog)?.books?.firstOrNull { book -> book.editions.any { it.id == edition.id } }
                owner?.let { book -> runCatching { EpubBook.open(File(local.path)).use { graph.chapterMarks.setCount(book.id, it.chapters.size) } } }
            }
            updateCatalog { withReading(it.copy(downloads = it.downloads + (edition.id to DownloadStatus.AVAILABLE), downloadProgress = it.downloadProgress - edition.id)) }
        } catch (error: Exception) {
            updateCatalog { it.copy(downloads = it.downloads + (edition.id to DownloadStatus.FAILED), downloadProgress = it.downloadProgress - edition.id, errors = it.errors + (edition.id to (error.message ?: "Download failed"))) }
        }
    }

    fun requestPasswordReset(email: String) {
        passwordResetUi = passwordResetUi.copy(working = true, error = null)
        scope.launch(Dispatchers.IO) {
            passwordResetUi = try {
                graph.library.requestPasswordReset(graph.session.serverUrl, email)
                passwordResetUi.copy(working = false, codeSent = true)
            } catch (error: Exception) {
                passwordResetUi.copy(working = false, error = error.message ?: "Couldn't send a reset code")
            }
        }
    }

    fun confirmPasswordReset(email: String, code: String, newPassword: String) {
        passwordResetUi = passwordResetUi.copy(working = true, error = null)
        scope.launch(Dispatchers.IO) {
            passwordResetUi = try {
                graph.library.confirmPasswordReset(graph.session.serverUrl, email, code, newPassword)
                passwordResetUi.copy(working = false, done = true)
            } catch (error: Exception) {
                passwordResetUi.copy(working = false, error = error.message ?: "Couldn't reset your password")
            }
        }
    }

    fun dismissPasswordReset() { passwordResetUi = PasswordResetUiState() }

    fun closeReader() {
        val closing = opened
        opened = null
        // The reader stays on screen while it fades out, so release the file after that.
        scope.launch { kotlinx.coroutines.delay(600); closing?.epub?.close() }
        refreshReading()
        scope.launch(Dispatchers.IO) { refreshSync() }
        ContinueReadingWidget.refresh(graph.context)
    }

    fun syncNow() {
        if (sync.running) return
        sync = sync.copy(running = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                graph.syncEngine.runOnce()
                graph.annotationSync.runOnce()
                sync = sync.copy(lastSyncMillis = System.currentTimeMillis())
            } catch (error: Exception) {
                sync = sync.copy(error = "Couldn't reach the server. Your progress is saved here and will sync when you're back online.")
            }
            refreshSync()
            refreshReading()
            ContinueReadingWidget.refresh(graph.context)
        }
    }

    private fun refreshSync() {
        sync = sync.copy(pending = unsynced(), rejected = graph.progress.rejectedCount(), running = false)
    }

    fun loadFriends() {
        friendsUi = friendsUi.copy(loading = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                val friends = graph.friends.friends()
                val requests = graph.friends.requests()
                val settings = graph.friends.settings()
                friendsUi = friendsUi.copy(friends = friends, incoming = requests.incoming, outgoing = requests.outgoing, settings = settings, loading = false)
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(loading = false, error = error.message ?: "Couldn't load friends")
            }
        }
    }

    fun sendFriendRequest(email: String, onSent: () -> Unit = {}) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.friends.sendRequest(email)
                loadFriends()
                onSent()
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(error = error.message ?: "Couldn't send that friend request")
            }
        }
    }

    fun acceptFriendRequest(userId: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.friends.accept(userId)
                loadFriends()
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(error = error.message ?: "Couldn't accept that request")
            }
        }
    }

    /** Declines a request someone sent you. The server call is the same as [cancelFriendRequest]; it tells them apart by who's asking. */
    fun declineFriendRequest(userId: String) = removeFriendRequest(userId, "Couldn't decline that request")

    /** Withdraws a request you sent. */
    fun cancelFriendRequest(userId: String) = removeFriendRequest(userId, "Couldn't cancel that request")

    private fun removeFriendRequest(userId: String, errorMessage: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.friends.removeRequest(userId)
                loadFriends()
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(error = error.message ?: errorMessage)
            }
        }
    }

    fun loadFriendProfile(userId: String) {
        if (friendProfile?.friend?.userId != userId) friendProfile = null
        scope.launch(Dispatchers.IO) {
            try {
                friendProfile = graph.friends.profile(userId)
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(error = error.message ?: "Couldn't load that profile")
            }
        }
    }

    fun removeFriend(userId: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.friends.removeFriend(userId)
                loadFriends()
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(error = error.message ?: "Couldn't remove that friend")
            }
        }
    }

    fun updateSocialSettings(visible: Boolean, goalYear: Int, goalBooks: Int) {
        scope.launch(Dispatchers.IO) {
            try {
                val settings = graph.friends.updateSettings(visible, goalYear, goalBooks)
                friendsUi = friendsUi.copy(settings = settings)
            } catch (error: Exception) {
                friendsUi = friendsUi.copy(error = error.message ?: "Couldn't update your sharing settings")
            }
        }
    }

    fun loadBookRequests() {
        bookRequestsUi = bookRequestsUi.copy(loading = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                val mine = graph.bookRequests.mine()
                bookRequestsUi = bookRequestsUi.copy(mine = mine, loading = false)
            } catch (error: Exception) {
                bookRequestsUi = bookRequestsUi.copy(loading = false, error = error.message ?: "Couldn't load your requests")
            }
        }
    }

    fun searchForRequest(query: String) {
        if (query.isBlank()) { bookRequestsUi = bookRequestsUi.copy(searchResults = emptyList(), searching = false); return }
        bookRequestsUi = bookRequestsUi.copy(searching = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                val results = graph.bookRequests.search(query)
                bookRequestsUi = bookRequestsUi.copy(searchResults = results, searching = false)
            } catch (error: Exception) {
                bookRequestsUi = bookRequestsUi.copy(searching = false, error = error.message ?: "Search failed")
            }
        }
    }

    fun requestBook(candidate: MetadataCandidate) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.bookRequests.request(candidate)
                bookRequestsUi = bookRequestsUi.copy(searchResults = emptyList(), notice = "Requested \"${candidate.title}\".")
                loadBookRequests()
            } catch (error: Exception) {
                bookRequestsUi = bookRequestsUi.copy(error = error.message ?: "Couldn't request that book")
            }
        }
    }

    fun cancelBookRequest(id: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.bookRequests.cancel(id)
                loadBookRequests()
            } catch (error: Exception) {
                bookRequestsUi = bookRequestsUi.copy(error = error.message ?: "Couldn't cancel that request")
            }
        }
    }

    fun dismissBookRequestNotice() { bookRequestsUi = bookRequestsUi.copy(notice = null) }

    fun loadLists() {
        listsUi = listsUi.copy(loading = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                val lists = graph.lists.lists()
                listsUi = listsUi.copy(lists = lists, loading = false)
            } catch (error: Exception) {
                listsUi = listsUi.copy(loading = false, error = error.message ?: "Couldn't load your lists")
            }
        }
    }

    fun createList(name: String, onCreated: (BookList) -> Unit = {}) {
        scope.launch(Dispatchers.IO) {
            try {
                val list = graph.lists.create(name)
                loadLists()
                onCreated(list)
            } catch (error: Exception) {
                (error.message ?: "Couldn't create that list").let { listsUi = listsUi.copy(error = it); notice = it }
            }
        }
    }

    fun renameList(id: String, name: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.rename(id, name)
                loadLists()
            } catch (error: Exception) {
                (error.message ?: "Couldn't rename that list").let { listsUi = listsUi.copy(error = it); notice = it }
            }
        }
    }

    fun deleteList(id: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.delete(id)
                if (listsUi.openList?.id == id) listsUi = listsUi.copy(openList = null, booksInOpenList = emptyList())
                loadLists()
            } catch (error: Exception) {
                (error.message ?: "Couldn't delete that list").let { listsUi = listsUi.copy(error = it); notice = it }
            }
        }
    }

    fun openList(list: BookList) {
        listsUi = listsUi.copy(openList = list, booksInOpenList = emptyList(), loading = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                val books = graph.lists.books(list.id)
                listsUi = listsUi.copy(booksInOpenList = books, loading = false)
            } catch (error: Exception) {
                listsUi = listsUi.copy(loading = false, error = error.message ?: "Couldn't load that list")
            }
        }
    }

    fun closeList() { listsUi = listsUi.copy(openList = null, booksInOpenList = emptyList()) }

    /** Adding is idempotent server-side, so this never needs to check membership first. */
    /** Shows a membership change at once; the reload after the server call reconciles it. */
    private fun optimisticMembership(listId: String, bookId: String, member: Boolean) {
        listsUi = listsUi.copy(lists = listsUi.lists.map { list ->
            if (list.id != listId || (bookId in list.bookIds) == member) list
            else if (member) list.copy(bookIds = listOf(bookId) + list.bookIds, bookCount = list.bookCount + 1)
            else list.copy(bookIds = list.bookIds - bookId, bookCount = (list.bookCount - 1).coerceAtLeast(0))
        })
    }

    fun addBookToList(listId: String, book: Book) {
        optimisticMembership(listId, book.id, true)
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.addBook(listId, book.id)
                if (listsUi.openList?.id == listId && listsUi.booksInOpenList.none { it.id == book.id }) listsUi = listsUi.copy(booksInOpenList = listsUi.booksInOpenList + book)
                loadLists()
            } catch (error: Exception) {
                (error.message ?: "Couldn't add that book to the list").let { listsUi = listsUi.copy(error = it); notice = it }
            }
        }
    }

    fun removeBookFromList(listId: String, bookId: String) {
        optimisticMembership(listId, bookId, false)
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.removeBook(listId, bookId)
                if (listsUi.openList?.id == listId) listsUi = listsUi.copy(booksInOpenList = listsUi.booksInOpenList.filter { it.id != bookId })
                loadLists()
            } catch (error: Exception) {
                (error.message ?: "Couldn't remove that book from the list").let { listsUi = listsUi.copy(error = it); notice = it }
            }
        }
    }

    /** Clears the last save's outcome, so a newly opened profile dialog starts fresh. */
    fun resetProfileStatus() { profileUi = ProfileUiState() }

    fun updateDisplayName(name: String, onSuccess: () -> Unit = {}) {
        profileUi = profileUi.copy(saving = true, error = null)
        scope.launch(Dispatchers.IO) {
            try {
                graph.library.updateSelf(displayName = name)
                profileUi = profileUi.copy(saving = false)
                onSuccess()
            } catch (error: Exception) {
                profileUi = profileUi.copy(saving = false, error = error.message ?: "Couldn't update your name")
            }
        }
    }

    fun changePassword(currentPassword: String, newPassword: String, onSuccess: () -> Unit = {}) {
        profileUi = profileUi.copy(saving = true, error = null, passwordChanged = false)
        scope.launch(Dispatchers.IO) {
            try {
                graph.library.updateSelf(currentPassword = currentPassword, newPassword = newPassword)
                profileUi = profileUi.copy(saving = false, passwordChanged = true)
                onSuccess()
            } catch (error: Exception) {
                profileUi = profileUi.copy(saving = false, error = error.message ?: "Couldn't change your password")
            }
        }
    }
}
