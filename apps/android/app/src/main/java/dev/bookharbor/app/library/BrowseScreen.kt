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
    var query by rememberSaveable { mutableStateOf("") }
    var sort by rememberSaveable { mutableStateOf(BookSort.Recent) }
    var viewMode by rememberSaveable { mutableStateOf(LibraryViewMode.Grid) }
    var tag by rememberSaveable { mutableStateOf<String?>(null) }
    val books = catalog.books
    val tags = remember(books) { libraryTags(books) }
    val arrivals = remember(books) { newArrivals(books) }
    val series = remember(books) { seriesGroups(books) }
    val searching = query.isNotBlank() || tag != null
    val shown = remember(books, query, sort, tag) { visibleBooks(books, query, ShelfFilter.All, sort, emptyMap(), emptySet(), tag) }
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
                    LibrarySearchBar(query, { query = it }, sort, { sort = it }, viewMode, onToggleView = {
                        viewMode = if (viewMode == LibraryViewMode.Grid) LibraryViewMode.List else LibraryViewMode.Grid
                    }, placeholder = "Search the catalogue")
                }
            }
            if (tags.isNotEmpty()) item(span = fullRow) {
                Box(Modifier.padding(top = 14.dp)) {
                    ChipRow { tags.forEach { option -> TagChip(option, tag == option) { tag = if (tag == option) null else option } } }
                }
            }
            if (!searching) {
                if (arrivals.isNotEmpty()) item(span = fullRow, key = "arrivals") { CoverRow("New arrivals", arrivals, controller, catalog) }
                series.forEach { (name, members) ->
                    item(span = fullRow, key = "series:$name") { CoverRow(name, members, controller, catalog, subtitle = "${members.size} ${if (members.size == 1) "book" else "books"}") }
                }
                item(span = fullRow) { SectionHeader("All books") }
            } else item(span = fullRow) { SectionHeader("${shown.size} ${if (shown.size == 1) "result" else "results"}") }

            when {
                books.isEmpty() -> item(span = fullRow) { EmptyState(BrandIcons.Library, "No books yet", "Ask your server's administrator to import a book, then pull to refresh.") }
                shown.isEmpty() -> item(span = fullRow) { EmptyState(BrandIcons.Search, "Nothing matches", "Try a shorter search, or request the book from the Requests tab.") }
                viewMode == LibraryViewMode.List -> items(shown, key = { it.id }, span = { fullRow() }, contentType = { "row" }) { book -> Box(Modifier.animateItem()) { BookRow(controller, book, catalog.cardState(book, downloaded)) } }
                else -> items(shown, key = { it.id }, contentType = { "cell" }) { book -> Box(Modifier.animateItem()) { BookGridCell(controller, book, catalog.cardState(book, downloaded), showOwned = true) } }
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
