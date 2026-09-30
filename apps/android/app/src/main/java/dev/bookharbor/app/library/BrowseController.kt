package dev.bookharbor.app.library

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class BrowseState(
    val filter: CatalogFilter = CatalogFilter(),
    val books: List<Book> = emptyList(),
    val total: Int = 0,
    val nextCursor: String? = null,
    val loading: Boolean = true,
    val error: String? = null,
    val libraries: List<CatalogLibrary> = emptyList(),
    val savedFilters: List<SavedCatalogFilter> = emptyList(),
    val saving: Boolean = false,
    val filterError: String? = null,
    val revision: Int = 0,
)

/** Search results never replace the complete catalog used for offline reading and Home. */
class BrowseController(private val client: CatalogClient, private val scope: CoroutineScope) {
    var state by mutableStateOf(BrowseState())
        private set
    private var generation = 0
    private var optionsJob: Job? = null
    private var mutationJob: Job? = null
    private var pageJob: Job? = null

    fun reset() {
        generation++
        optionsJob?.cancel(); mutationJob?.cancel(); pageJob?.cancel()
        state = BrowseState()
    }

    fun setFilter(filter: CatalogFilter) {
        generation++
        pageJob?.cancel()
        state = state.copy(filter = filter, books = emptyList(), total = 0, nextCursor = null, loading = true, error = null, revision = state.revision + 1)
    }

    suspend fun search(cached: List<Book>, offline: Boolean) {
        val request = ++generation
        pageJob?.cancel()
        val filter = state.filter
        state = state.copy(loading = true, error = null, nextCursor = null)
        try {
            if (offline) {
                val books = cachedCatalogSearch(cached, filter)
                if (request == generation) state = state.copy(books = books, total = books.size, loading = false)
            } else {
                delay(350)
                val page = withContext(Dispatchers.IO) { client.search(filter) }
                if (request == generation) state = state.copy(books = page.books, total = page.total, nextCursor = page.nextCursor, loading = false)
            }
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            if (request == generation) state = state.copy(books = emptyList(), total = 0, loading = false, error = error.message ?: "Search failed. Try again.")
        }
    }

    fun loadMore() {
        if (state.loading) return
        val cursor = state.nextCursor ?: return
        val request = generation
        val filter = state.filter
        state = state.copy(loading = true, error = null)
        pageJob = scope.launch {
            try {
                val page = withContext(Dispatchers.IO) { client.search(filter, cursor) }
                if (request == generation) state = state.copy(books = (state.books + page.books).distinctBy { it.id }, total = page.total, nextCursor = page.nextCursor, loading = false)
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) {
                if (request == generation) state = state.copy(loading = false, error = error.message ?: "Unable to load more books. Try again.")
            }
        }
    }

    fun loadOptions() {
        optionsJob?.cancel()
        optionsJob = scope.launch {
            try {
                val libraries = withContext(Dispatchers.IO) { client.libraries() }
                state = state.copy(libraries = libraries)
                val filters = withContext(Dispatchers.IO) { client.filters() }
                state = state.copy(savedFilters = filters, filterError = null)
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) { state = state.copy(filterError = error.message ?: "Unable to load saved filters. Try again.") }
        }
    }

    fun saveFilter(name: String, id: String? = null, onSaved: () -> Unit) {
        val filter = state.filter
        mutate(onSaved) {
            val saved = withContext(Dispatchers.IO) { client.saveFilter(name, filter, id) }
            state.copy(savedFilters = (state.savedFilters.filterNot { it.id == saved.id } + saved).sortedBy { it.name.lowercase() })
        }
    }

    fun deleteFilter(id: String, onDeleted: () -> Unit) = mutate(onDeleted) {
        withContext(Dispatchers.IO) { client.deleteFilter(id) }
        state.copy(savedFilters = state.savedFilters.filterNot { it.id == id })
    }

    fun clearFilterError() { state = state.copy(filterError = null) }

    private fun mutate(onSuccess: () -> Unit, action: suspend () -> BrowseState) {
        if (state.saving) return
        state = state.copy(saving = true, filterError = null)
        mutationJob = scope.launch {
            try {
                // The action runs on Main; only HTTP goes onto IO, so concurrent search updates
                // aren't overwritten by a snapshot captured on a worker thread.
                state = action().copy(saving = false)
                onSuccess()
            } catch (error: CancellationException) { throw error
            } catch (error: Exception) { state = state.copy(saving = false, filterError = error.message ?: "Unable to save filter. Try again.") }
        }
    }
}
