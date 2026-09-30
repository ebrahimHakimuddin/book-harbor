package dev.bookharbor.app.library

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyGridItemSpanScope
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.EmptyState
import dev.bookharbor.app.ui.ScreenTitle
import dev.bookharbor.app.ui.SectionHeader

/**
 * Browse: everything on the server. Without a search it's arranged for discovery -- new
 * arrivals, each series in order, tags -- then every book; a search or tag turns it into results.
 */
@Composable
internal fun BrowseTab(controller: AppController, catalog: LibraryUiState.Catalog, reselected: Int = 0) {
    val gridState = androidx.compose.foundation.lazy.grid.rememberLazyGridState()
    androidx.compose.runtime.LaunchedEffect(reselected) { if (reselected > 0) gridState.animateScrollToItem(0) }
    var sort by rememberSaveable { mutableStateOf(BookSort.Recent) }
    var viewMode by rememberSaveable { mutableStateOf(LibraryViewMode.Grid) }
    val browse = controller.browse
    val state = browse.state
    val filter = state.filter
    LaunchedEffect(filter, state.revision, catalog.books, catalog.offline, controller.refreshing) {
        if (!controller.refreshing) browse.search(catalog.books, catalog.offline)
    }
    LaunchedEffect(catalog.offline, controller.refreshing) {
        if (!catalog.offline && !controller.refreshing) browse.loadOptions()
    }
    val books = catalog.books
    val tags = remember(books) { libraryTags(books) }
    val arrivals = remember(books) { newArrivals(books) }
    val series = remember(books) { seriesGroups(books) }
    val searching = filter != CatalogFilter()
    val shown = remember(state.books, sort) { visibleBooks(state.books, "", ShelfFilter.All, sort, emptyMap(), emptySet()) }
    val downloaded = remember(catalog.downloads) { catalog.downloadedEditions() }
    val fullRow: LazyGridItemSpanScope.() -> GridItemSpan = { GridItemSpan(maxLineSpan) }

    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(if (viewMode == LibraryViewMode.Grid) 3 else 1),
            state = gridState,
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(horizontal = 20.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            item(span = fullRow) { ScreenTitle("Browse", "${books.size} ${if (books.size == 1) "book" else "books"} on ${catalog.instanceName}") }
            item(span = fullRow) {
                Box(Modifier.padding(top = 14.dp)) {
                    LibrarySearchBar(filter.q, { browse.setFilter(filter.copy(q = it.take(300))) }, sort, { sort = it }, viewMode, onToggleView = {
                        viewMode = if (viewMode == LibraryViewMode.Grid) LibraryViewMode.List else LibraryViewMode.Grid
                    }, placeholder = "Search the catalogue")
                }
            }
            item(span = fullRow) { BrowseFilters(browse, catalog.offline, series.map { it.first }) }
            if (tags.isNotEmpty()) item(span = fullRow) {
                Box(Modifier.padding(top = 14.dp)) {
                    ChipRow { tags.forEach { option -> TagChip(option, filter.tag == option) { browse.setFilter(filter.copy(tag = if (filter.tag == option) "" else option)) } } }
                }
            }
            if (!searching) {
                if (arrivals.isNotEmpty()) item(span = fullRow, key = "arrivals") { CoverRow("New arrivals", arrivals, controller, catalog) }
                series.forEach { (name, members) ->
                    item(span = fullRow, key = "series:$name") { CoverRow(name, members, controller, catalog, subtitle = "${members.size} ${if (members.size == 1) "book" else "books"}") }
                }
                item(span = fullRow) { SectionHeader("All books") }
            } else item(span = fullRow) { SectionHeader("${state.total} ${if (state.total == 1) "result" else "results"}") }

            if (catalog.offline) item(span = fullRow) { Text("Offline · searching the last saved catalog", Modifier.padding(vertical = 12.dp), style = MaterialTheme.typography.bodySmall) }
            if (state.nextCursor != null && sort != BookSort.Recent) item(span = fullRow) { Text("Sorting ${shown.size} loaded books of ${state.total}", style = MaterialTheme.typography.bodySmall) }
            if (state.error != null) item(span = fullRow) {
                androidx.compose.foundation.layout.Column {
                    Text(state.error, color = MaterialTheme.colorScheme.error)
                    TextButton(onClick = { if (state.nextCursor != null) browse.loadMore() else browse.setFilter(filter.copy()) }) { Text("Retry search") }
                }
            }

            when {
                state.loading && shown.isEmpty() -> item(span = fullRow) { Box(Modifier.padding(24.dp)) { CircularProgressIndicator() } }
                state.error != null && shown.isEmpty() -> Unit
                books.isEmpty() -> item(span = fullRow) { EmptyState(BrandIcons.Library, "No books yet", "Ask your server's administrator to import a book, then pull to refresh.") }
                shown.isEmpty() -> item(span = fullRow) { EmptyState(BrandIcons.Search, "Nothing matches", "Try a shorter search, or request the book from the Requests tab.") }
                viewMode == LibraryViewMode.List -> items(shown, key = { it.id }, span = { fullRow() }, contentType = { "row" }) { book -> Box(Modifier.animateItem()) { BookRow(controller, book, catalog.cardState(book, downloaded)) } }
                else -> items(shown, key = { it.id }, contentType = { "cell" }) { book -> Box(Modifier.animateItem()) { BookGridCell(controller, book, catalog.cardState(book, downloaded), showOwned = true) } }
            }
            if (state.nextCursor != null) item(span = fullRow) {
                TextButton(onClick = browse::loadMore, enabled = !state.loading, modifier = Modifier.padding(vertical = 12.dp)) {
                    Text(if (state.loading) "Loading more…" else "Load more · ${shown.size} of ${state.total}")
                }
            }
        }
    }
}

/** A titled row of covers that scrolls sideways, bleeding to the screen edges. */
@Composable
private fun CoverRow(title: String, books: List<Book>, controller: AppController, catalog: LibraryUiState.Catalog, subtitle: String? = null) {
    val downloaded = remember(catalog.downloads) { catalog.downloadedEditions() }
    androidx.compose.foundation.layout.Column {
        SectionHeader(if (subtitle != null) "$title · $subtitle" else title)
        LazyRow(
            Modifier.bleed(20.dp),
            contentPadding = PaddingValues(horizontal = 20.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            items(books, key = { it.id }) { book -> BookShelfCard(controller, book, catalog.cardState(book, downloaded)) }
        }
    }
}
