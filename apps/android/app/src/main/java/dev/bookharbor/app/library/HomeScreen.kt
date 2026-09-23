package dev.bookharbor.app.library

import android.content.Intent
import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Switch
import androidx.core.content.ContextCompat
import android.net.Uri
import androidx.compose.foundation.BorderStroke
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.border
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.layout
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.Crossfade
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.material3.ripple
import androidx.compose.ui.composed
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import dev.bookharbor.app.ui.pressScale
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyGridItemSpanScope
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import kotlinx.coroutines.launch
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.bookharbor.app.R
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.EmptyState
import dev.bookharbor.app.ui.PrimaryButton
import dev.bookharbor.app.ui.SectionHeader
import dev.bookharbor.app.ui.theme.HarborNavy
import dev.bookharbor.app.ui.theme.LiterataFamily
import dev.bookharbor.app.ui.theme.cautionColor
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.text.DateFormat
import java.util.Date
import kotlin.math.roundToInt

internal enum class LibraryViewMode { List, Grid }

@Composable
/**
 * Home: the reader's own shelf -- books downloaded to this device or started anywhere -- with
 * search, sort, shelves, and tags over just those. The whole catalogue lives in Browse.
 */
internal fun HomeTab(controller: AppController, catalog: LibraryUiState.Catalog, onOpenHistory: () -> Unit, onOpenLists: () -> Unit, onOpenSettings: () -> Unit, onBrowse: () -> Unit, reselected: Int = 0) {
    var query by rememberSaveable { mutableStateOf("") }
    var sort by rememberSaveable { mutableStateOf(BookSort.Recent) }
    var viewMode by rememberSaveable { mutableStateOf(LibraryViewMode.Grid) }
    var tag by rememberSaveable { mutableStateOf<String?>(null) }
    var listId by rememberSaveable { mutableStateOf<String?>(null) }
    LaunchedEffect(Unit) { controller.loadLists() }
    val lists = controller.listsUi.lists
    val selectedList = lists.firstOrNull { it.id == listId }
    val downloaded = remember(catalog.downloads) { catalog.downloadedEditions() }
    val shelf = remember(catalog.books, catalog.progress, downloaded) { catalog.books.filter { isOnShelf(it, catalog.progress, downloaded) } }
    // A chosen list replaces the shelf as the source, so it shows every book in it, started or not.
    val source = remember(shelf, catalog.books, selectedList) { selectedList?.bookIds?.toSet()?.let { ids -> catalog.books.filter { it.id in ids } } ?: shelf }
    val tags = remember(source) { libraryTags(source) }
    // A tag chosen before switching lists may not exist in the new source; it would filter
    // invisibly (its chip is gone), so it only applies while it is one of the shown tags.
    val activeTag = tag?.takeIf { it in tags }
    val searching = query.isNotBlank() || activeTag != null || selectedList != null
    val shown = remember(source, selectedList, query, sort, catalog.progress, downloaded, activeTag) {
        val books = visibleBooks(source, query, ShelfFilter.All, sort, catalog.progress, downloaded, activeTag)
        // A list keeps its own order (newest added first) under the default sort.
        if (selectedList != null && sort == BookSort.Recent) selectedList.bookIds.withIndex().associate { it.value to it.index }.let { order -> books.sortedBy { order[it.id] } } else books
    }
    val hero = if (searching) null else remember(catalog.books, catalog.progress, catalog.lastReadAt) { continueReading(catalog.books, catalog.progress, catalog.lastReadAt) }
    // Shelves are sections rather than filter chips: in progress, downloaded but not started, and
    // finished. The book in the Continue card isn't repeated under Reading.
    val sections = remember(shown, catalog.progress, downloaded, hero) { homeSections(shown.filter { it.id != hero?.id }, catalog.progress, downloaded) }
    val fullRow: LazyGridItemSpanScope.() -> GridItemSpan = { GridItemSpan(maxLineSpan) }
    val gridState = androidx.compose.foundation.lazy.grid.rememberLazyGridState()
    // Tapping Home while already on it returns to the top.
    LaunchedEffect(reselected) { if (reselected > 0) gridState.animateScrollToItem(0) }

    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    LazyVerticalGrid(
        columns = GridCells.Fixed(if (viewMode == LibraryViewMode.Grid) 3 else 1),
        state = gridState,
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(horizontal = 20.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        item(span = fullRow, contentType = "top") { LibraryTopBar(controller, catalog, onOpenHistory, onOpenLists, onOpenSettings) }
        // Search and filters come first, above everything they act on.
        if (shelf.isNotEmpty()) item(span = fullRow, contentType = "search") {
            Box(Modifier.padding(top = 16.dp)) {
                LibrarySearchBar(query, { query = it }, sort, { sort = it }, viewMode, onToggleView = {
                    viewMode = if (viewMode == LibraryViewMode.Grid) LibraryViewMode.List else LibraryViewMode.Grid
                })
            }
        }
        // Lists narrow Home to one list's books; tapping the active list clears it.
        if (lists.isNotEmpty() && catalog.books.isNotEmpty()) item(span = fullRow, contentType = "lists") {
            Box(Modifier.padding(top = 12.dp)) { ChipRow { lists.forEach { list -> ShelfChip(list.name, list.bookCount, list.id == listId) { listId = if (list.id == listId) null else list.id } } } }
        }
        // Tags narrow the shelf; tapping the active tag clears it.
        if (tags.isNotEmpty()) item(span = fullRow, contentType = "tags") {
            Box(Modifier.padding(top = 12.dp)) { ChipRow { tags.forEach { option -> TagChip(option, activeTag == option) { tag = if (activeTag == option) null else option } } } }
        }
        if (catalog.offline) item(span = fullRow, contentType = "offline") {
            Row(Modifier.fillMaxWidth().padding(top = 12.dp).clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surfaceVariant).padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(BrandIcons.CloudOff, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Text("Offline. Showing your last saved library; downloaded books open normally.", Modifier.padding(start = 10.dp).weight(1f), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                TextButton(onClick = controller::load) { Text("Retry") }
            }
        }
        hero?.let { book ->
            item(span = fullRow, key = "continue", contentType = "continue") { ContinueReadingCard(controller, book, catalog.progress[book.id] ?: 0.0, catalog.chaptersLeft[book.id]) }
        }
        when {
            shelf.isEmpty() && selectedList == null -> item(span = fullRow) {
                EmptyState(
                    BrandIcons.Library, "Your shelf is empty",
                    if (catalog.books.isEmpty()) "Your server has no books yet. Ask its administrator to import some, then pull to refresh."
                    else "Books you download or start reading live here. Find something in the catalogue to begin.",
                    action = if (catalog.books.isNotEmpty()) { { PrimaryButton("Browse the catalogue", onBrowse, icon = BrandIcons.Search) } } else null,
                )
            }
            searching && shown.isEmpty() -> item(span = fullRow) { EmptyState(BrandIcons.Search, "Nothing matches", if (selectedList != null && selectedList.bookIds.isEmpty()) "This list is empty." else "Try a different search, tag, or list.") }
            searching -> {
                item(span = fullRow, contentType = "header") { SectionHeader("${shown.size} ${if (shown.size == 1) "book" else "books"}") }
                bookItems(controller, catalog, shown, downloaded, viewMode, fullRow)
            }
            else -> sections.forEach { (section, books) ->
                item(span = fullRow, key = "section:${section.name}", contentType = "header") { SectionHeader("${section.title} · ${books.size}") }
                bookItems(controller, catalog, books, downloaded, viewMode, fullRow)
            }
        }
    }
    }
}

/** A run of books as grid cells or list rows, each card given only its own precomputed state. */
private fun androidx.compose.foundation.lazy.grid.LazyGridScope.bookItems(
    controller: AppController,
    catalog: LibraryUiState.Catalog,
    books: List<Book>,
    downloaded: Set<String>,
    viewMode: LibraryViewMode,
    fullRow: LazyGridItemSpanScope.() -> GridItemSpan,
) {
    if (viewMode == LibraryViewMode.List) {
        items(books, key = { it.id }, span = { fullRow() }, contentType = { "row" }) { book -> Box(Modifier.animateItem()) { BookRow(controller, book, catalog.cardState(book, downloaded)) } }
    } else {
        items(books, key = { it.id }, contentType = { "cell" }) { book -> Box(Modifier.animateItem()) { BookGridCell(controller, book, catalog.cardState(book, downloaded)) } }
    }
}

/** The book in progress, one tap from the top of the library. */
@Composable
internal fun ContinueReadingCard(controller: AppController, book: Book, progress: Double, chaptersLeft: Int?) {
    val press = remember { MutableInteractionSource() }
    val shown by animateFloatAsState(progress.toFloat().coerceIn(0f, 1f), tween(600), label = "continue")
    Row(
        Modifier.fillMaxWidth().padding(top = 18.dp).clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surfaceVariant)
            .pressScale(press, 0.98f).clickable(press, ripple(), onClickLabel = "Continue reading ${book.title}", role = Role.Button) { controller.open(book) }.padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Cover(book, controller.covers, Modifier.width(56.dp))
        Column(Modifier.weight(1f).padding(horizontal = 14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("CONTINUE READING", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary, letterSpacing = 1.2.sp)
            Text(book.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurface)
            chaptersLeft?.let { Text(chaptersLeftLabel(it), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.weight(1f).height(4.dp).clip(CircleShape).background(MaterialTheme.colorScheme.background)) {
                    Box(Modifier.fillMaxWidth(shown).fillMaxHeight().background(MaterialTheme.colorScheme.secondary))
                }
                Text("${(progress * 100).roundToInt()}%", Modifier.padding(start = 10.dp), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

/**
 * A compact title bar: the page name where the eye lands first, then sync status, lists, and
 * the account -- the brand mark stays small instead of taking over the top of the library.
 */
@Composable
private fun LibraryTopBar(controller: AppController, catalog: LibraryUiState.Catalog, onOpenHistory: () -> Unit, onOpenLists: () -> Unit, onOpenSettings: () -> Unit) {
    val sync = controller.sync
    Row(Modifier.fillMaxWidth().padding(top = 12.dp), verticalAlignment = Alignment.CenterVertically) {
        Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.size(30.dp))
        Column(Modifier.weight(1f).padding(start = 10.dp)) {
            Text("Home", style = MaterialTheme.typography.headlineMedium, maxLines = 1)
            Text(catalog.instanceName, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        val (icon, description) = when {
            catalog.offline -> BrandIcons.CloudOff to "Offline. Open reading history."
            sync.pending > 0 -> BrandIcons.Cloud to "${sync.pending} updates waiting to sync. Open reading history."
            else -> BrandIcons.CloudDone to "Everything synced. Open reading history."
        }
        dev.bookharbor.app.ui.IconAction(icon, description, onOpenHistory)
        dev.bookharbor.app.ui.IconAction(BrandIcons.Bookmark, "Your lists", onOpenLists)
        Box(
            Modifier.padding(start = 4.dp).size(36.dp).clip(CircleShape).background(MaterialTheme.colorScheme.secondary)
                .clickable(onClickLabel = "Open settings", role = Role.Button, onClick = onOpenSettings),
            contentAlignment = Alignment.Center,
        ) { Text(initialsOf(controller.displayName), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSecondary) }
    }
}

/**
 * One pill for finding books: search on the left, then sort and the list/grid switch inside the
 * same surface, so the three read as a single control rather than three mismatched boxes.
 */
@Composable
internal fun LibrarySearchBar(query: String, onQuery: (String) -> Unit, sort: BookSort, onSort: (BookSort) -> Unit, viewMode: LibraryViewMode, onToggleView: () -> Unit, placeholder: String = "Search your books") {
    var menu by remember { mutableStateOf(false) }
    val muted = MaterialTheme.colorScheme.onSurfaceVariant
    Row(
        Modifier.fillMaxWidth().height(52.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant).padding(start = 18.dp, end = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(BrandIcons.Search, null, Modifier.size(20.dp), tint = muted)
        BasicTextField(
            query, onQuery, Modifier.weight(1f).padding(horizontal = 12.dp).semantics { contentDescription = "Search books, authors, series, and tags" },
            singleLine = true,
            textStyle = MaterialTheme.typography.bodyLarge.copy(color = MaterialTheme.colorScheme.onSurface),
            cursorBrush = SolidColor(MaterialTheme.colorScheme.secondary),
            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
            decorationBox = { field ->
                Box(contentAlignment = Alignment.CenterStart) {
                    if (query.isEmpty()) Text(placeholder, style = MaterialTheme.typography.bodyLarge, color = muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    field()
                }
            },
        )
        AnimatedVisibility(query.isNotEmpty(), enter = fadeIn() + scaleIn(), exit = fadeOut() + scaleOut()) {
            IconButton(onClick = { onQuery("") }) { Icon(BrandIcons.Close, "Clear search", Modifier.size(18.dp), tint = muted) }
        }
        Box(Modifier.width(1.dp).height(24.dp).background(muted.copy(alpha = 0.3f)))
        Box {
            dev.bookharbor.app.ui.IconAction(BrandIcons.Filter, "Sort: ${sort.label}", { menu = true }, tint = if (sort != BookSort.Recent) MaterialTheme.colorScheme.secondary else muted, iconSize = 20.dp)
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                Text("Sort by", Modifier.padding(horizontal = 16.dp, vertical = 8.dp), style = MaterialTheme.typography.labelMedium, color = muted)
                BookSort.entries.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(option.label, fontWeight = if (option == sort) FontWeight.SemiBold else FontWeight.Normal) },
                        trailingIcon = if (option == sort) { { Icon(BrandIcons.Check, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.secondary) } } else null,
                        onClick = { onSort(option); menu = false },
                    )
                }
            }
        }
        dev.bookharbor.app.ui.IconAction(
            if (viewMode == LibraryViewMode.Grid) BrandIcons.List else BrandIcons.Grid,
            if (viewMode == LibraryViewMode.Grid) "Show as a list" else "Show as a grid", onToggleView, tint = muted, iconSize = 20.dp,
        )
    }
}

/** A chip row that bleeds past the grid's 20dp margin to the screen edges, keeping the margin as scroll padding. */
@Composable
internal fun ChipRow(content: @Composable () -> Unit) {
    Row(
        Modifier.bleed(20.dp).horizontalScroll(rememberScrollState()).padding(horizontal = 20.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) { content() }
}

/** Widens a full-row item by [margin] on each side, cancelling the parent's content padding. */
internal fun Modifier.bleed(margin: androidx.compose.ui.unit.Dp) = layout { measurable, constraints ->
    val extra = margin.roundToPx() * 2
    val placeable = measurable.measure(constraints.copy(minWidth = constraints.maxWidth + extra, maxWidth = constraints.maxWidth + extra))
    layout(constraints.maxWidth, placeable.height) { placeable.place(-extra / 2, 0) }
}

@Composable
internal fun ShelfChip(label: String, count: Int, selected: Boolean, onClick: () -> Unit) {
    val container by animateColorAsState(if (selected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.6f), tween(180), label = "shelf")
    val content by animateColorAsState(if (selected) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface, tween(180), label = "shelf text")
    Row(
        Modifier.height(38.dp).clip(CircleShape).background(container)
            .selectable(selected, role = Role.Tab, onClick = onClick).padding(horizontal = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.labelLarge, color = content, maxLines = 1)
        Text("  $count", style = MaterialTheme.typography.labelLarge, color = content.copy(alpha = 0.6f), maxLines = 1)
    }
}

@Composable
internal fun TagChip(tag: String, selected: Boolean, onClick: () -> Unit) {
    val border by animateColorAsState(if (selected) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.outline.copy(alpha = 0.45f), tween(180), label = "tag")
    Row(
        Modifier.height(30.dp).clip(CircleShape).border(1.dp, border, CircleShape)
            .background(if (selected) MaterialTheme.colorScheme.secondary.copy(alpha = 0.14f) else Color.Transparent)
            .selectable(selected, role = Role.Checkbox, onClick = onClick).padding(horizontal = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text("#", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
        Text(tag, style = MaterialTheme.typography.labelMedium, color = if (selected) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
    }
}

