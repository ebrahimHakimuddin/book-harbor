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
    data class SignIn(val instance: InstanceInfo? = null, val serverUrl: String = "", val email: String = "") : LibraryUiState
    data object Loading : LibraryUiState
    data class Catalog(
        val instanceName: String,
        val books: List<Book>,
        val downloads: Map<String, DownloadStatus> = emptyMap(),
        val errors: Map<String, String> = emptyMap(),
        val progress: Map<String, Double> = emptyMap(),
        /** RFC 3339 UTC instant of each book's most recent local reading event, for the History tab. */
        val lastReadAt: Map<String, String> = emptyMap(),
        /** True when showing the last saved catalog because the server could not be reached. */
        val offline: Boolean = false,
    ) : LibraryUiState
    data class Error(val message: String, val retry: LibraryUiState) : LibraryUiState
}

data class SyncUiState(val pending: Int = 0, val rejected: Int = 0, val running: Boolean = false, val error: String? = null, val lastSyncMillis: Long = 0)

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
    var bookRequestsUi by mutableStateOf(BookRequestsUiState())
        private set
    var listsUi by mutableStateOf(ListsUiState())
        private set
    /** Set when signing out would discard reading updates that have not reached the server. */
    var unsyncedOnSignOut by mutableStateOf<Int?>(null)
        private set
    var notice by mutableStateOf<String?>(null)
    val covers = CoverLoader(graph.api, graph.cacheDir)
    val serverUrl: String get() = graph.session.serverUrl
    val displayName: String get() = graph.session.displayName

    private val cache = CatalogCache(graph.prefs)

    /** True while a pull-to-refresh reload is running; the catalog stays on screen meanwhile. */
    var refreshing by mutableStateOf(false)
        private set

    fun refresh() {
        if (refreshing) return
        refreshing = true
        load(quiet = true)
    }

    fun load(quiet: Boolean = false) {
        if (!quiet) ui = LibraryUiState.Loading
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
            refreshing = false
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
                ui = LibraryUiState.Error(message, LibraryUiState.SignIn(serverUrl = graph.session.serverUrl, email = email))
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
            ui = when (signOutTarget) {
                SignOutTarget.SIGN_IN -> LibraryUiState.SignIn(serverUrl = graph.session.serverUrl)
                SignOutTarget.SETUP -> { graph.session.serverUrl = ""; LibraryUiState.Setup }
            }
            refreshSync()
        }
    }

    /** Signs out and lands on server setup, so the reader can point the app at a different server. */
    fun switchServer(force: Boolean = false) = signOut(force, SignOutTarget.SETUP)

    /** Confirms the sign-out [unsyncedOnSignOut] warned about, keeping whichever target the original attempt asked for. */
    fun confirmSignOut() = signOut(force = true, target = signOutTarget)

    fun dismissSignOutWarning() { unsyncedOnSignOut = null }

    private fun catalog(name: String, books: List<Book>, offline: Boolean): LibraryUiState.Catalog {
        val available = graph.downloads.all().map { it.editionId }.toSet()
        val (progress, lastReadAt) = readingProgress()
        return LibraryUiState.Catalog(
            instanceName = name,
            books = books,
            downloads = books.flatMap { it.editions }.associate { it.id to if (it.id in available) DownloadStatus.AVAILABLE else DownloadStatus.NOT_DOWNLOADED },
            progress = progress,
            lastReadAt = lastReadAt,
            offline = offline,
        )
    }

    /** Percentage and last-activity time per book, from this device's local reading positions. */
    private fun readingProgress(): Pair<Map<String, Double>, Map<String, String>> {
        val positions = graph.progress.positions()
        return positions.associate { it.bookId to it.percentage } to positions.associate { it.bookId to it.occurredAt }
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
        updateCatalog { val (progress, lastReadAt) = readingProgress(); it.copy(progress = progress, lastReadAt = lastReadAt) }
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
            updateCatalog { val (progress, lastReadAt) = readingProgress(); it.copy(progress = progress, lastReadAt = lastReadAt) }
        }
    }

    private fun refreshSync() {
        sync = sync.copy(pending = graph.progress.pendingCount(), rejected = graph.progress.rejectedCount(), running = false)
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
                listsUi = listsUi.copy(error = error.message ?: "Couldn't create that list")
            }
        }
    }

    fun renameList(id: String, name: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.rename(id, name)
                loadLists()
            } catch (error: Exception) {
                listsUi = listsUi.copy(error = error.message ?: "Couldn't rename that list")
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
                listsUi = listsUi.copy(error = error.message ?: "Couldn't delete that list")
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
    fun addBookToList(listId: String, book: Book) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.addBook(listId, book.id)
                if (listsUi.openList?.id == listId) listsUi = listsUi.copy(booksInOpenList = listsUi.booksInOpenList + book)
                loadLists()
            } catch (error: Exception) {
                listsUi = listsUi.copy(error = error.message ?: "Couldn't add that book to the list")
            }
        }
    }

    fun removeBookFromList(listId: String, bookId: String) {
        scope.launch(Dispatchers.IO) {
            try {
                graph.lists.removeBook(listId, bookId)
                if (listsUi.openList?.id == listId) listsUi = listsUi.copy(booksInOpenList = listsUi.booksInOpenList.filter { it.id != bookId })
                loadLists()
            } catch (error: Exception) {
                listsUi = listsUi.copy(error = error.message ?: "Couldn't remove that book from the list")
            }
        }
    }

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
