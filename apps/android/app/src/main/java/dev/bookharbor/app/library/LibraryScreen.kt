package dev.bookharbor.app.library

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
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
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.bookharbor.app.R
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.theme.HarborNavy
import dev.bookharbor.app.ui.theme.LiterataFamily
import dev.bookharbor.app.ui.theme.cautionColor
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.text.DateFormat
import java.util.Date
import kotlin.math.roundToInt

private enum class Tab(val label: String) { Library("Library"), History("History"), Friends("Friends"), Requests("Requests"), More("More") }

@Composable
fun LibraryScreen(controller: AppController) {
    when (val state = controller.ui) {
        LibraryUiState.Setup -> EntryColumn {
            Text("Connect to your BookHarbor server to begin.", style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
            ServerForm(initial = controller.serverUrl, onConnect = controller::connect)
        }
        is LibraryUiState.SignIn -> EntryColumn(subtitle = state.instance?.name) {
            Text(state.serverUrl.removePrefix("https://").removePrefix("http://"), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary)
            SignInForm(initialEmail = state.email, onSignIn = controller::signIn)
            TextButton(onClick = controller::changeServer) { Text("Use a different server") }
        }
        LibraryUiState.Loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(16.dp)) {
                Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.height(72.dp))
                CircularProgressIndicator(color = MaterialTheme.colorScheme.secondary)
                Text("Opening your library…", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        is LibraryUiState.Error -> EntryColumn {
            Text(state.message, style = MaterialTheme.typography.bodyLarge, textAlign = TextAlign.Center, color = MaterialTheme.colorScheme.error)
            Button(onClick = { if (state.retry == LibraryUiState.Loading) controller.load() else controller.dismissError(state.retry) }, shape = RoundedCornerShape(9.dp)) { Text("Try again") }
        }
        is LibraryUiState.Catalog -> CatalogScaffold(controller, state)
    }
}

/** The brand column used before you are signed in: mark, wordmark, tagline, then the form. */
@Composable
private fun EntryColumn(subtitle: String? = null, content: @Composable () -> Unit) {
    Column(
        Modifier.fillMaxSize().statusBarsPadding().verticalScroll(rememberScrollState()).padding(horizontal = 28.dp, vertical = 40.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(18.dp),
    ) {
        Spacer(Modifier.height(24.dp))
        Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.height(96.dp))
        Text(subtitle ?: "BookHarbor", style = MaterialTheme.typography.headlineLarge, color = MaterialTheme.colorScheme.onBackground)
        Spacer(Modifier.height(8.dp))
        content()
    }
}

@Composable
private fun ServerForm(initial: String, onConnect: (String) -> Unit) {
    var value by remember(initial) { mutableStateOf(initial) }
    OutlinedTextField(value, { value = it }, label = { Text("Server address") }, placeholder = { Text("https://books.example.com") }, singleLine = true,
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri), shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth())
    // Home-network servers are often plain http; allow it, but say what that means.
    if (value.trim().startsWith("http://", ignoreCase = true)) {
        Text("This address isn't encrypted. Your password and books can be seen by others on the network, so only use it at home or on a network you trust.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.error)
    }
    Button(onClick = { onConnect(value) }, enabled = value.isNotBlank(), shape = RoundedCornerShape(9.dp), modifier = Modifier.fillMaxWidth().height(48.dp)) { Text("Connect") }
}

@Composable
private fun SignInForm(initialEmail: String, onSignIn: (String, String) -> Unit) {
    var email by remember(initialEmail) { mutableStateOf(initialEmail) }
    var password by remember { mutableStateOf("") }
    OutlinedTextField(email, { email = it }, label = { Text("Email") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email), shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth())
    OutlinedTextField(password, { password = it }, label = { Text("Password") }, singleLine = true, visualTransformation = PasswordVisualTransformation(), keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password), shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth())
    Button(onClick = { onSignIn(email.trim(), password) }, enabled = email.isNotBlank() && password.isNotBlank(), shape = RoundedCornerShape(9.dp), modifier = Modifier.fillMaxWidth().height(48.dp)) { Text("Sign in") }
}

@Composable
private fun CatalogScaffold(controller: AppController, catalog: LibraryUiState.Catalog) {
    var tab by rememberSaveable { mutableStateOf(Tab.Library) }
    var listsOpen by rememberSaveable { mutableStateOf(false) }
    if (listsOpen) {
        ListsScreen(controller, onClose = { listsOpen = false })
        return
    }
    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                Tab.entries.forEach { item ->
                    NavigationBarItem(
                        selected = tab == item,
                        onClick = { tab = item },
                        icon = { Icon(when (item) { Tab.Library -> BrandIcons.Library; Tab.History -> BrandIcons.History; Tab.Friends -> BrandIcons.Friends; Tab.Requests -> BrandIcons.Request; Tab.More -> BrandIcons.More }, contentDescription = null) },
                        label = { Text(item.label) },
                        colors = NavigationBarItemDefaults.colors(selectedIconColor = MaterialTheme.colorScheme.onPrimary, selectedTextColor = MaterialTheme.colorScheme.primary, indicatorColor = MaterialTheme.colorScheme.primary),
                    )
                }
            }
        },
    ) { padding ->
        Box(Modifier.padding(padding).fillMaxSize()) {
            when (tab) {
                Tab.Library -> LibraryTab(controller, catalog, onOpenHistory = { tab = Tab.History }, onOpenLists = { listsOpen = true })
                Tab.History -> HistoryTab(controller, catalog)
                Tab.Friends -> FriendsTab(controller)
                Tab.Requests -> RequestsTab(controller)
                Tab.More -> MoreTab(controller, catalog)
            }
        }
    }
    controller.unsyncedOnSignOut?.let { count ->
        AlertDialog(
            onDismissRequest = controller::dismissSignOutWarning,
            title = { Text("Some changes haven't synced") },
            text = { Text("$count reading ${if (count == 1) "update or note hasn't" else "updates or notes haven't"} reached your server yet. Signing out now discards ${if (count == 1) "it" else "them"}. Connect to the internet and sync first to keep your place and highlights.") },
            confirmButton = { TextButton(onClick = controller::confirmSignOut) { Text("Sign out anyway", color = MaterialTheme.colorScheme.error) } },
            dismissButton = { TextButton(onClick = controller::dismissSignOutWarning) { Text("Keep me signed in") } },
        )
    }
}

// ---- Library ---------------------------------------------------------------------------------

private enum class LibraryViewMode { List, Grid }

@Composable
private fun LibraryTab(controller: AppController, catalog: LibraryUiState.Catalog, onOpenHistory: () -> Unit, onOpenLists: () -> Unit) {
    var query by rememberSaveable { mutableStateOf("") }
    var filter by rememberSaveable { mutableStateOf(ShelfFilter.All) }
    var sort by rememberSaveable { mutableStateOf(BookSort.Recent) }
    var viewMode by rememberSaveable { mutableStateOf(LibraryViewMode.Grid) }
    val downloaded = remember(catalog.downloads) { catalog.downloads.filterValues { it == DownloadStatus.AVAILABLE }.keys }
    val shown = remember(catalog.books, query, filter, sort, catalog.progress, downloaded) { visibleBooks(catalog.books, query, filter, sort, catalog.progress, downloaded) }
    val fullRow: LazyGridItemSpanScope.() -> GridItemSpan = { GridItemSpan(maxLineSpan) }

    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    LazyVerticalGrid(
        columns = GridCells.Fixed(if (viewMode == LibraryViewMode.Grid) 3 else 1),
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(horizontal = 20.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        item(span = fullRow) { LibraryHeader(controller, catalog, onOpenHistory) }
        item(span = fullRow) {
            Row(Modifier.fillMaxWidth().padding(top = 18.dp, bottom = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Text("Library", Modifier.weight(1f), style = MaterialTheme.typography.headlineLarge)
                TextButton(onClick = onOpenLists) { Text("Your lists") }
            }
        }
        item(span = fullRow) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Box(Modifier.weight(1f)) { SearchRow(query, { query = it }, sort) { sort = it } }
                // Matches the search field and sort button's height (56dp); IconButton's 48dp
                // default made this row visibly uneven.
                IconButton(onClick = { viewMode = if (viewMode == LibraryViewMode.Grid) LibraryViewMode.List else LibraryViewMode.Grid }, modifier = Modifier.size(56.dp)) {
                    Icon(if (viewMode == LibraryViewMode.Grid) BrandIcons.List else BrandIcons.Grid, if (viewMode == LibraryViewMode.Grid) "Switch to list view" else "Switch to grid view")
                }
            }
        }
        item(span = fullRow) {
            Row(Modifier.padding(vertical = 14.dp).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                ShelfFilter.entries.forEach { option ->
                    FilterChip(
                        selected = filter == option,
                        onClick = { filter = option },
                        label = { Text(option.label) },
                        shape = CircleShape,
                        colors = FilterChipDefaults.filterChipColors(selectedContainerColor = MaterialTheme.colorScheme.primary, selectedLabelColor = MaterialTheme.colorScheme.onPrimary, containerColor = Color.Transparent),
                    )
                }
            }
        }
        if (catalog.offline) item(span = fullRow) {
            Row(Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surfaceVariant).padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
                Icon(BrandIcons.CloudOff, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                Text("Offline. Showing your last saved library; downloaded books open normally.", Modifier.padding(start = 10.dp).weight(1f), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                TextButton(onClick = controller::load) { Text("Retry") }
            }
        }
        if (shown.isEmpty()) {
            item(span = fullRow) { EmptyShelf(hasBooks = catalog.books.isNotEmpty(), filtering = query.isNotBlank() || filter != ShelfFilter.All) }
        } else if (viewMode == LibraryViewMode.List) {
            items(shown, key = { it.id }, span = { fullRow() }) { book -> BookRow(controller, catalog, book) }
        } else {
            items(shown, key = { it.id }) { book -> BookGridCell(controller, catalog, book) }
        }
    }
    }
}

@Composable
private fun LibraryHeader(controller: AppController, catalog: LibraryUiState.Catalog, onOpenHistory: () -> Unit) {
    val sync = controller.sync
    // Three equal-weight columns instead of a Box overlay: the brand mark and the actions can
    // never collide, even at large system font sizes, because each is bounded to its own third.
    Row(Modifier.fillMaxWidth().padding(top = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Spacer(Modifier.weight(1f))
        Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally) {
            Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.height(38.dp))
            Text("BookHarbor", fontFamily = LiterataFamily, fontWeight = FontWeight.Bold, fontSize = 17.sp, color = MaterialTheme.colorScheme.onBackground, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Row(Modifier.weight(1f), horizontalArrangement = Arrangement.End, verticalAlignment = Alignment.CenterVertically) {
            val (icon, description) = when {
                catalog.offline -> BrandIcons.CloudOff to "Offline. Open reading history."
                sync.pending > 0 -> BrandIcons.Cloud to "${sync.pending} updates waiting to sync. Open reading history."
                else -> BrandIcons.CloudDone to "Everything synced. Open reading history."
            }
            IconButton(onClick = onOpenHistory) { Icon(icon, description, tint = MaterialTheme.colorScheme.onBackground) }
            Box(Modifier.size(34.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant), contentAlignment = Alignment.Center) {
                Text(initialsOf(controller.displayName), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onBackground)
            }
        }
    }
}

@Composable
private fun SearchRow(query: String, onQuery: (String) -> Unit, sort: BookSort, onSort: (BookSort) -> Unit) {
    var menu by remember { mutableStateOf(false) }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        OutlinedTextField(
            value = query, onValueChange = onQuery, singleLine = true, modifier = Modifier.weight(1f),
            placeholder = { Text("Search books, authors…") },
            leadingIcon = { Icon(BrandIcons.Search, null, Modifier.size(20.dp)) },
            trailingIcon = if (query.isNotEmpty()) { { IconButton(onClick = { onQuery("") }) { Icon(BrandIcons.Close, "Clear search", Modifier.size(18.dp)) } } } else null,
            shape = RoundedCornerShape(12.dp),
        )
        Box {
            OutlinedButton(onClick = { menu = true }, shape = RoundedCornerShape(12.dp), contentPadding = PaddingValues(0.dp), modifier = Modifier.size(56.dp)) {
                Icon(BrandIcons.Filter, "Sort books. Now: ${sort.label}")
            }
            DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                BookSort.entries.forEach { option ->
                    DropdownMenuItem(
                        text = { Text(option.label, fontWeight = if (option == sort) FontWeight.Bold else FontWeight.Normal) },
                        trailingIcon = if (option == sort) { { Icon(BrandIcons.Check, null, Modifier.size(18.dp)) } } else null,
                        onClick = { onSort(option); menu = false },
                    )
                }
            }
        }
    }
}

@Composable
private fun BookRow(controller: AppController, catalog: LibraryUiState.Catalog, book: Book) {
    val progress = catalog.progress[book.id]
    val statuses = book.editions.map { catalog.downloads[it.id] ?: DownloadStatus.NOT_DOWNLOADED }
    val downloading = statuses.any { it == DownloadStatus.DOWNLOADING }
    val error = book.editions.firstNotNullOfOrNull { edition -> catalog.errors[edition.id]?.takeIf { catalog.downloads[edition.id] == DownloadStatus.FAILED || it.isNotBlank() } }
    Column {
        Row(
            Modifier.fillMaxWidth().clickable(onClickLabel = "Open ${book.title}", role = Role.Button) { controller.open(book) }.padding(vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Cover(book, controller.covers, Modifier.width(52.dp))
            Column(Modifier.weight(1f).padding(horizontal = 14.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(book.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onBackground)
                val by = book.authors.joinToString(", ").ifBlank { book.subtitle }
                if (by.isNotBlank()) Text(by, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                when {
                    downloading -> Row(verticalAlignment = Alignment.CenterVertically) {
                        CircularProgressIndicator(Modifier.size(12.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.secondary)
                        Text("  Downloading…", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
                    }
                    error != null -> Text(error, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.error, maxLines = 2)
                    else -> Text(statusLine(progress, statuses.any { it == DownloadStatus.AVAILABLE }), style = MaterialTheme.typography.labelMedium, color = if (isReading(progress) || isFinished(progress)) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            BookMenu(controller, catalog, book)
        }
        HorizontalDivider(Modifier.padding(start = 66.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.35f))
    }
}

@Composable
private fun BookGridCell(controller: AppController, catalog: LibraryUiState.Catalog, book: Book) {
    val progress = catalog.progress[book.id]
    val downloaded = book.editions.any { catalog.downloads[it.id] == DownloadStatus.AVAILABLE }
    Column(Modifier.fillMaxWidth().padding(bottom = 16.dp)) {
        Box {
            Cover(
                book, controller.covers,
                Modifier.fillMaxWidth().clickable(onClickLabel = "Open ${book.title}", role = Role.Button) { controller.open(book) },
            )
            // A scrim behind the menu button, since the icon's fixed tint would otherwise vanish
            // against whatever color the cover art happens to be.
            Box(
                Modifier.align(Alignment.TopEnd).padding(2.dp).clip(CircleShape).background(Color.Black.copy(alpha = 0.35f)),
            ) { BookMenu(controller, catalog, book, tint = Color.White) }
            if (isReading(progress)) {
                Box(Modifier.align(Alignment.BottomStart).fillMaxWidth().height(3.dp).background(Color.Black.copy(alpha = 0.25f))) {
                    Box(Modifier.fillMaxWidth((progress ?: 0.0).toFloat()).fillMaxHeight().background(MaterialTheme.colorScheme.secondary))
                }
            }
        }
        Text(book.title, Modifier.padding(top = 6.dp), style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onBackground)
        Text(
            if (isFinished(progress)) "Finished" else if (downloaded) "Available offline" else "Tap to download",
            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1,
        )
    }
}

private fun statusLine(progress: Double?, offline: Boolean): String {
    val place = when {
        isFinished(progress) -> "Finished"
        isReading(progress) -> "Reading · ${((progress ?: 0.0) * 100).roundToInt()}%"
        else -> null
    }
    return listOfNotNull(place, if (offline) "Available offline" else "Tap to download").joinToString(" · ").let { if (place == null && offline) "Available offline" else it }
}

@Composable
private fun BookMenu(controller: AppController, catalog: LibraryUiState.Catalog, book: Book, tint: Color = MaterialTheme.colorScheme.onSurfaceVariant) {
    var open by remember { mutableStateOf(false) }
    var confirmRemove by remember { mutableStateOf<Edition?>(null) }
    var addToListOpen by remember { mutableStateOf(false) }
    Box {
        val hasAction = book.editions.any { catalog.downloads[it.id] != DownloadStatus.DOWNLOADING }
        IconButton(onClick = { open = true }, enabled = hasAction) { Icon(BrandIcons.MoreVertical, "More options for ${book.title}", tint = if (hasAction) tint else tint.copy(alpha = 0.38f)) }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(
                text = { Text("Add to list") },
                leadingIcon = { Icon(BrandIcons.Request, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                onClick = { open = false; addToListOpen = true },
            )
            book.editions.forEach { edition ->
                val format = edition.format.uppercase().ifBlank { "Edition" }
                val status = catalog.downloads[edition.id]
                // Downloading isn't cancellable yet, so don't offer an action that would just
                // restart the same edition from zero mid-transfer.
                if (status == DownloadStatus.DOWNLOADING) return@forEach
                val available = status == DownloadStatus.AVAILABLE
                // Removing a download is reversible (just refetch it), so it gets the milder
                // caution tone rather than colorScheme.error, which is reserved for actions
                // that lose data or a relationship (e.g. removing a friend, signing out unsynced).
                val caution = cautionColor()
                DropdownMenuItem(
                    text = { Text(if (available) "Remove $format download" else "Download $format", color = if (available) caution else Color.Unspecified) },
                    leadingIcon = { Icon(if (available) BrandIcons.Trash else BrandIcons.Download, null, Modifier.size(18.dp), tint = if (available) caution else MaterialTheme.colorScheme.onSurfaceVariant) },
                    onClick = { open = false; if (available) confirmRemove = edition else controller.download(edition) },
                )
            }
        }
    }
    confirmRemove?.let { edition ->
        AlertDialog(
            onDismissRequest = { confirmRemove = null },
            title = { Text("Remove download?") },
            text = { Text("\"${book.title}\" (${edition.format.uppercase()}) will be deleted from this device. You can download it again any time.") },
            confirmButton = { TextButton(onClick = { controller.removeDownload(edition); confirmRemove = null }) { Text("Remove", color = cautionColor()) } },
            dismissButton = { TextButton(onClick = { confirmRemove = null }) { Text("Cancel") } },
        )
    }
    if (addToListOpen) AddToListDialog(controller, book, onDismiss = { addToListOpen = false })
}

@Composable
private fun AddToListDialog(controller: AppController, book: Book, onDismiss: () -> Unit) {
    LaunchedEffect(Unit) { controller.loadLists() }
    val lists = controller.listsUi.lists
    var newListName by rememberSaveable { mutableStateOf<String?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add \"${book.title}\" to a list") },
        text = {
            Column {
                if (lists.isEmpty()) Text("No lists yet.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                lists.forEach { list ->
                    TextButton(onClick = { controller.addBookToList(list.id, book); onDismiss() }, modifier = Modifier.fillMaxWidth()) {
                        Text(list.name, Modifier.weight(1f), textAlign = TextAlign.Start)
                    }
                }
                TextButton(onClick = { newListName = "" }, modifier = Modifier.fillMaxWidth()) { Text("New list…", Modifier.weight(1f), textAlign = TextAlign.Start) }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text("Close") } },
    )
    if (newListName != null) {
        var name by rememberSaveable(newListName) { mutableStateOf(newListName.orEmpty()) }
        AlertDialog(
            onDismissRequest = { newListName = null },
            title = { Text("New list") },
            text = { OutlinedTextField(value = name, onValueChange = { name = it }, singleLine = true, label = { Text("Name") }, modifier = Modifier.fillMaxWidth()) },
            confirmButton = {
                TextButton(
                    onClick = {
                        controller.createList(name.trim()) { created -> controller.addBookToList(created.id, book) }
                        newListName = null
                        onDismiss()
                    },
                    enabled = name.isNotBlank(),
                ) { Text("Create") }
            },
            dismissButton = { TextButton(onClick = { newListName = null }) { Text("Cancel") } },
        )
    }
}

@Composable
internal fun Cover(book: Book, loader: CoverLoader, modifier: Modifier = Modifier) {
    val bitmap by produceState<ImageBitmap?>(null, book.coverUrl, book.updatedAt) { value = withContext(Dispatchers.IO) { loader.load(book) } }
    Box(
        modifier.aspectRatio(2f / 3f).clip(RoundedCornerShape(6.dp)).background(Brush.linearGradient(listOf(HarborNavy, Color(0xFF315B72)))),
        contentAlignment = Alignment.Center,
    ) {
        val image = bitmap
        if (image != null) Image(image, contentDescription = null, Modifier.fillMaxSize(), contentScale = ContentScale.Crop)
        else Text(initialsOf(book.title), fontFamily = LiterataFamily, fontWeight = FontWeight.Bold, fontSize = 17.sp, color = Color.White)
    }
}

@Composable
private fun EmptyShelf(hasBooks: Boolean, filtering: Boolean) {
    Column(Modifier.fillMaxWidth().padding(vertical = 48.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Image(painterResource(R.drawable.brand_mark), contentDescription = null, Modifier.height(64.dp), alpha = 0.55f)
        Text(if (hasBooks && filtering) "Nothing here yet" else "No books yet", style = MaterialTheme.typography.titleLarge)
        Text(
            if (hasBooks && filtering) "Try a different search, or pick another shelf." else "Ask your server's administrator to import a book, then pull to refresh.",
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center,
        )
    }
}

// ---- History ---------------------------------------------------------------------------------

@Composable
private fun HistoryTab(controller: AppController, catalog: LibraryUiState.Catalog) {
    val sync = controller.sync
    val synced = sync.pending == 0 && !catalog.offline && sync.error == null
    val read = remember(catalog.books, catalog.lastReadAt) {
        catalog.books.filter { it.id in catalog.lastReadAt }.sortedByDescending { catalog.lastReadAt[it.id] }
    }
    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(horizontal = 20.dp, vertical = 24.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        item { Text("History", style = MaterialTheme.typography.headlineLarge, modifier = Modifier.padding(bottom = 14.dp)) }
        item {
            Row(
                Modifier.fillMaxWidth().padding(bottom = 18.dp).clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surfaceVariant)
                    .clickable(onClick = controller::syncNow, enabled = !sync.running).padding(16.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (sync.running) CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.secondary)
                else Icon(if (synced) BrandIcons.CloudDone else if (catalog.offline) BrandIcons.CloudOff else BrandIcons.Cloud, null, Modifier.size(22.dp), tint = MaterialTheme.colorScheme.secondary)
                Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
                    Text(
                        when {
                            sync.running -> "Syncing…"
                            catalog.offline -> "You're offline"
                            sync.pending > 0 -> "${sync.pending} ${if (sync.pending == 1) "update" else "updates"} waiting"
                            else -> "Everything is synced"
                        },
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        sync.error ?: if (sync.lastSyncMillis > 0) "Last synced ${DateFormat.getTimeInstance(DateFormat.SHORT).format(Date(sync.lastSyncMillis))}" else "Tap to sync now",
                        style = MaterialTheme.typography.bodySmall, color = if (sync.error != null) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Icon(BrandIcons.Sync, "Sync now", tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (sync.rejected > 0) item {
            Text(
                "${sync.rejected} reading ${if (sync.rejected == 1) "update was" else "updates were"} refused by the server and won't be retried.",
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(bottom = 14.dp),
            )
        }
        if (read.isEmpty()) {
            item {
                Column(Modifier.fillMaxWidth().padding(vertical = 48.dp), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    Icon(BrandIcons.History, null, Modifier.size(40.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text("Nothing read yet", style = MaterialTheme.typography.titleLarge)
                    Text("Books you open will show up here.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
                }
            }
        } else {
            items(read, key = { it.id }) { book -> HistoryRow(controller, catalog, book) }
        }
    }
    }
}

@Composable
private fun HistoryRow(controller: AppController, catalog: LibraryUiState.Catalog, book: Book) {
    val progress = catalog.progress[book.id]
    val when_ = catalog.lastReadAt[book.id]?.let(::relativeReadTime)
    Row(
        Modifier.fillMaxWidth().clickable(onClickLabel = "Open ${book.title}", role = Role.Button) { controller.open(book) }.padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Cover(book, controller.covers, Modifier.width(52.dp))
        Column(Modifier.weight(1f).padding(horizontal = 14.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Text(book.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onBackground)
            val status = when {
                isFinished(progress) -> "Finished"
                isReading(progress) -> "${((progress ?: 0.0) * 100).roundToInt()}% read"
                else -> "Started"
            }
            Text(
                listOfNotNull(status, when_).joinToString(" · "),
                style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** A short "3h ago" / "2d ago" / date label from an RFC 3339 UTC instant. */
private fun relativeReadTime(occurredAt: String): String? {
    val then = runCatching { java.time.Instant.parse(occurredAt) }.getOrNull() ?: return null
    val minutes = java.time.Duration.between(then, java.time.Instant.now()).toMinutes()
    return when {
        minutes < 1 -> "Just now"
        minutes < 60 -> "${minutes}m ago"
        minutes < 24 * 60 -> "${minutes / 60}h ago"
        minutes < 7 * 24 * 60 -> "${minutes / (24 * 60)}d ago"
        else -> DateFormat.getDateInstance(DateFormat.MEDIUM).format(Date.from(then))
    }
}

// ---- More ------------------------------------------------------------------------------------

@Composable
private fun MoreTab(controller: AppController, catalog: LibraryUiState.Catalog) {
    PullToRefreshBox(isRefreshing = controller.refreshing, onRefresh = controller::refresh, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(24.dp), verticalArrangement = Arrangement.spacedBy(18.dp)) {
        Text("More", style = MaterialTheme.typography.headlineLarge)

        var confirmSignOut by rememberSaveable { mutableStateOf(false) }
        var confirmSwitchServer by rememberSaveable { mutableStateOf(false) }
        // Identity and the session actions that act on it (change server, sign out) live in one
        // card -- previously the sign-out/change-server buttons floated below the Profile card,
        // disconnected from the account they act on.
        SettingsCard {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(44.dp).clip(CircleShape).background(MaterialTheme.colorScheme.background), contentAlignment = Alignment.Center) {
                    Text(initialsOf(controller.displayName), style = MaterialTheme.typography.titleMedium)
                }
                Column(Modifier.padding(start = 14.dp)) {
                    Text(controller.displayName.ifBlank { "Signed in" }, style = MaterialTheme.typography.titleMedium)
                    Text(catalog.instanceName, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(BrandIcons.Server, null, Modifier.size(20.dp), tint = MaterialTheme.colorScheme.secondary)
                Text(controller.serverUrl, Modifier.padding(start = 12.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f))
            TextButton(onClick = { confirmSwitchServer = true }, modifier = Modifier.fillMaxWidth()) { Text("Change server") }
            OutlinedButton(onClick = { confirmSignOut = true }, shape = RoundedCornerShape(9.dp), modifier = Modifier.fillMaxWidth().height(48.dp)) {
                Icon(BrandIcons.SignOut, null, Modifier.size(18.dp))
                Text("  Sign out")
            }
        }
        ProfileSection(controller)
        if (confirmSignOut) {
            AlertDialog(
                onDismissRequest = { confirmSignOut = false },
                title = { Text("Sign out?") },
                text = { Text("You'll need your email and password to sign back in.") },
                confirmButton = { TextButton(onClick = { confirmSignOut = false; controller.signOut() }) { Text("Sign out", color = MaterialTheme.colorScheme.error) } },
                dismissButton = { TextButton(onClick = { confirmSignOut = false }) { Text("Cancel") } },
            )
        }
        if (confirmSwitchServer) {
            AlertDialog(
                onDismissRequest = { confirmSwitchServer = false },
                title = { Text("Change server?") },
                text = { Text("You'll be signed out of ${controller.serverUrl} and asked for a new server address.") },
                confirmButton = { TextButton(onClick = { confirmSwitchServer = false; controller.switchServer() }) { Text("Change server", color = MaterialTheme.colorScheme.error) } },
                dismissButton = { TextButton(onClick = { confirmSwitchServer = false }) { Text("Cancel") } },
            )
        }
        AboutSection()
    }
    }
}

@Composable
private fun SettingsCard(modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(
        modifier.fillMaxWidth().clip(RoundedCornerShape(16.dp)).background(MaterialTheme.colorScheme.surfaceVariant).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        content = content,
    )
}

@Composable
private fun ProfileSection(controller: AppController) {
    val state = controller.profileUi
    var name by rememberSaveable(controller.displayName) { mutableStateOf(controller.displayName) }
    var currentPassword by rememberSaveable { mutableStateOf("") }
    var newPassword by rememberSaveable { mutableStateOf("") }
    var confirmPassword by rememberSaveable { mutableStateOf("") }

    // The default unfocused text-field border and divider both use colorScheme.outline, which
    // is too close in luminance to this card's surfaceVariant background to read as a border.
    val fieldColors = OutlinedTextFieldDefaults.colors(unfocusedBorderColor = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f))
    SettingsCard {
        Text("Profile", style = MaterialTheme.typography.titleMedium)
        Row(verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(
                value = name, onValueChange = { name = it }, singleLine = true, modifier = Modifier.weight(1f),
                label = { Text("Display name") }, shape = RoundedCornerShape(12.dp), colors = fieldColors,
            )
            TextButton(
                onClick = { controller.updateDisplayName(name.trim()) },
                enabled = !state.saving && name.isNotBlank() && name.trim() != controller.displayName,
            ) { Text("Save") }
        }

        HorizontalDivider(color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.4f))
        Text("Change password", style = MaterialTheme.typography.titleSmall)
        OutlinedTextField(
            value = currentPassword, onValueChange = { currentPassword = it }, singleLine = true, modifier = Modifier.fillMaxWidth(),
            label = { Text("Current password") }, visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password), shape = RoundedCornerShape(12.dp), colors = fieldColors,
        )
        OutlinedTextField(
            value = newPassword, onValueChange = { newPassword = it }, singleLine = true, modifier = Modifier.fillMaxWidth(),
            label = { Text("New password") }, visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password), shape = RoundedCornerShape(12.dp), colors = fieldColors,
        )
        OutlinedTextField(
            value = confirmPassword, onValueChange = { confirmPassword = it }, singleLine = true, modifier = Modifier.fillMaxWidth(),
            label = { Text("Confirm new password") }, visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password), shape = RoundedCornerShape(12.dp),
            isError = confirmPassword.isNotEmpty() && confirmPassword != newPassword, colors = fieldColors,
        )
        if (state.error != null) Text(state.error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
        if (state.passwordChanged) Text("Password updated.", color = MaterialTheme.colorScheme.secondary, style = MaterialTheme.typography.bodySmall)
        Button(
            onClick = {
                controller.changePassword(currentPassword, newPassword) { currentPassword = ""; newPassword = ""; confirmPassword = "" }
            },
            enabled = !state.saving && currentPassword.isNotBlank() && newPassword.length >= 12 && newPassword == confirmPassword,
            shape = RoundedCornerShape(9.dp), modifier = Modifier.fillMaxWidth(),
        ) {
            if (state.saving) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onPrimary) else Text("Update password")
        }
    }
}

private sealed interface UpdateStatus {
    data object Idle : UpdateStatus
    data object Checking : UpdateStatus
    data object UpToDate : UpdateStatus
    data class Available(val release: Release) : UpdateStatus
    data object Failed : UpdateStatus
}

@Composable
private fun AboutSection() {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val installed = remember { context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty() }
    var status by remember { mutableStateOf<UpdateStatus>(UpdateStatus.Idle) }
    val open = { url: String -> context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) }

    SettingsCard {
        Text("About", style = MaterialTheme.typography.titleMedium)
        Text("BookHarbor $installed", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        TextButton(onClick = { open(GITHUB_URL) }, contentPadding = PaddingValues(0.dp)) { Text(GITHUB_URL.removePrefix("https://")) }
        Text(
            when (val s = status) {
                UpdateStatus.Idle -> ""
                UpdateStatus.Checking -> "Checking for updates…"
                UpdateStatus.UpToDate -> "You're on the latest version."
                is UpdateStatus.Available -> "Version ${s.release.version} is available."
                UpdateStatus.Failed -> "Couldn't check for updates. Try again when you're online."
            },
            Modifier.semantics { liveRegion = LiveRegionMode.Polite },
            style = MaterialTheme.typography.bodyMedium,
            color = if (status == UpdateStatus.Failed) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
        )
        val available = status as? UpdateStatus.Available
        OutlinedButton(
            onClick = {
                if (available != null) open(available.release.downloadUrl) else {
                    status = UpdateStatus.Checking
                    scope.launch {
                        status = try {
                            val latest = withContext(Dispatchers.IO) { latestRelease() }
                            if (isNewer(latest.version, installed)) UpdateStatus.Available(latest) else UpdateStatus.UpToDate
                        } catch (e: Exception) { UpdateStatus.Failed }
                    }
                }
            },
            enabled = status != UpdateStatus.Checking,
            shape = RoundedCornerShape(9.dp), modifier = Modifier.fillMaxWidth().height(48.dp),
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f)),
        ) { Text(if (available != null) "Download update" else "Check for updates") }
    }
}
