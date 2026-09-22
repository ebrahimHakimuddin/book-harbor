package dev.bookharbor.app.library

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyGridItemSpanScope
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.ripple
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import dev.bookharbor.app.ui.AppRow
import dev.bookharbor.app.ui.AppTextField
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.EmptyState
import dev.bookharbor.app.ui.GroupCard
import dev.bookharbor.app.ui.GroupShape
import dev.bookharbor.app.ui.PrimaryButton
import dev.bookharbor.app.ui.RowDivider
import dev.bookharbor.app.ui.ScreenTitle
import dev.bookharbor.app.ui.SecondaryButton
import dev.bookharbor.app.ui.pressScale

/** A reader's own named lists: a grid of lists shown by their covers, and each list's books. */
@Composable
fun ListsScreen(controller: AppController, onClose: () -> Unit) {
    LaunchedEffect(Unit) { controller.loadLists() }
    val state = controller.listsUi
    val catalog = controller.ui as? LibraryUiState.Catalog ?: return
    state.openList?.let { open ->
        BackHandler(onBack = controller::closeList)
        ListDetailScreen(controller, catalog, state.lists.firstOrNull { it.id == open.id } ?: open, state.booksInOpenList, onBack = controller::closeList)
        return
    }
    BackHandler(onBack = onClose)
    var naming by rememberSaveable { mutableStateOf<String?>(null) }
    val fullRow: LazyGridItemSpanScope.() -> GridItemSpan = { GridItemSpan(maxLineSpan) }
    val books = remember(catalog.books) { catalog.books.associateBy { it.id } }

    LazyVerticalGrid(
        GridCells.Fixed(2), Modifier.fillMaxSize().statusBarsPadding(),
        contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 32.dp),
        horizontalArrangement = Arrangement.spacedBy(14.dp), verticalArrangement = Arrangement.spacedBy(18.dp),
    ) {
        item(span = fullRow) {
            Column {
                IconButton(onClick = onClose, modifier = Modifier.padding(top = 8.dp).offset(x = (-12).dp)) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back to Home") }
                ScreenTitle("Lists", if (state.lists.isEmpty()) null else "${state.lists.size} ${if (state.lists.size == 1) "list" else "lists"}") {
                    TextButton(onClick = { naming = "" }) { Text("New list") }
                }
            }
        }
        if (state.error != null) item(span = fullRow) { Text(state.error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
        if (state.lists.isEmpty() && !state.loading) item(span = fullRow) {
            EmptyState(BrandIcons.Bookmark, "No lists yet", "Group books however you like: to read next, favourites, a book club. Add books from a book's quick actions.", action = {
                PrimaryButton("Create a list", onClick = { naming = "" })
            })
        }
        items(state.lists, key = { it.id }) { list ->
            ListCard(controller, list, list.bookIds.mapNotNull { books[it] }.take(3), onOpen = { controller.openList(list) })
        }
    }
    naming?.let { initial -> NameListDialog("New list", initial, "Create", onDone = { controller.createList(it); naming = null }, onDismiss = { naming = null }) }
}

/** A list shown as a fan of its first covers over its name and size. Press and hold to rename or delete. */
@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun ListCard(controller: AppController, list: BookList, covers: List<Book>, onOpen: () -> Unit) {
    val press = remember { MutableInteractionSource() }
    val haptics = LocalHapticFeedback.current
    var menu by remember { mutableStateOf(false) }
    var renaming by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    Column(
        Modifier.pressScale(press, 0.97f).combinedClickable(
            press, ripple(), role = Role.Button, onClickLabel = "Open ${list.name}", onLongClickLabel = "Rename or delete",
            onLongClick = { haptics.performHapticFeedback(HapticFeedbackType.LongPress); menu = true }, onClick = onOpen,
        ),
    ) {
        Box(Modifier.fillMaxWidth().aspectRatio(1.15f).clip(GroupShape).background(MaterialTheme.colorScheme.surfaceVariant), contentAlignment = Alignment.Center) {
            if (covers.isEmpty()) Icon(BrandIcons.Bookmark, null, Modifier.size(32.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f))
            // Up to three covers fanned out: the newest in front.
            covers.reversed().forEachIndexed { index, book ->
                val fromFront = covers.size - 1 - index
                Cover(
                    book, controller.covers,
                    Modifier.width(62.dp).offset(x = (fromFront * 22 - (covers.size - 1) * 11).dp).rotate((fromFront - (covers.size - 1) / 2f) * 6f)
                        .shadow(6.dp, androidx.compose.foundation.shape.RoundedCornerShape(6.dp)),
                )
            }
        }
        Text(list.name, Modifier.padding(top = 10.dp), style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
        Text("${list.bookCount} ${if (list.bookCount == 1) "book" else "books"}", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            DropdownMenuItem(text = { Text("Rename") }, onClick = { menu = false; renaming = true })
            DropdownMenuItem(text = { Text("Delete", color = MaterialTheme.colorScheme.error) }, onClick = { menu = false; confirmDelete = true })
        }
    }
    if (renaming) NameListDialog("Rename list", list.name, "Save", onDone = { controller.renameList(list.id, it); renaming = false }, onDismiss = { renaming = false })
    if (confirmDelete) DeleteListDialog(list, onConfirm = { controller.deleteList(list.id); confirmDelete = false }, onDismiss = { confirmDelete = false })
}

@Composable
private fun ListDetailScreen(controller: AppController, catalog: LibraryUiState.Catalog, list: BookList, books: List<Book>, onBack: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    var renaming by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val fullRow: LazyGridItemSpanScope.() -> GridItemSpan = { GridItemSpan(maxLineSpan) }
    val missing = books.count { book -> book.editions.none { catalog.downloads[it.id] == DownloadStatus.AVAILABLE } }
    LazyVerticalGrid(
        GridCells.Fixed(1), Modifier.fillMaxSize().statusBarsPadding(),
        contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 32.dp),
    ) {
        item(span = fullRow) {
            Row(Modifier.padding(top = 8.dp).fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack, modifier = Modifier.offset(x = (-12).dp)) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back to your lists") }
                Box(Modifier.weight(1f))
                Box {
                    IconButton(onClick = { menu = true }) { Icon(BrandIcons.MoreVertical, "List options") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(text = { Text("Rename") }, onClick = { menu = false; renaming = true })
                        DropdownMenuItem(text = { Text("Delete list", color = MaterialTheme.colorScheme.error) }, onClick = { menu = false; confirmDelete = true })
                    }
                }
            }
        }
        item(span = fullRow) { ScreenTitle(list.name, "${books.size} ${if (books.size == 1) "book" else "books"}") }
        if (books.isNotEmpty()) item(span = fullRow) {
            SecondaryButton(
                if (missing == 0) "All downloaded" else "Download $missing ${if (missing == 1) "book" else "books"}",
                onClick = { controller.downloadAll(books) }, enabled = missing > 0, icon = BrandIcons.Download,
                modifier = Modifier.fillMaxWidth().padding(top = 16.dp, bottom = 16.dp),
            )
        }
        if (books.isEmpty() && !controller.listsUi.loading) item(span = fullRow) {
            EmptyState(BrandIcons.Bookmark, "Nothing here yet", "Add books to this list from a book's quick actions: press and hold it, or tap its \"⋮\".")
        } else item(span = fullRow) {
            GroupCard {
                books.forEachIndexed { index, book ->
                    var details by remember { mutableStateOf(false) }
                    if (details) BookDetailsSheet(controller, catalog, book, onDismiss = { details = false })
                    AppRow(
                        book.title, listOfNotNull(book.authors.firstOrNull(), seriesLabel(book).takeIf { it.isNotBlank() }).joinToString(" · ").ifBlank { null },
                        leading = { Cover(book, controller.covers, Modifier.width(44.dp)) },
                        onClick = { details = true },
                        trailing = {
                            IconButton(onClick = { controller.removeBookFromList(list.id, book.id); controller.notice = "Removed from ${list.name}" }) {
                                Icon(BrandIcons.Close, "Remove ${book.title} from this list", Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        },
                    )
                    if (index < books.lastIndex) RowDivider(inset = 74.dp)
                }
            }
        }
    }
    if (renaming) NameListDialog("Rename list", list.name, "Save", onDone = { controller.renameList(list.id, it); renaming = false }, onDismiss = { renaming = false })
    if (confirmDelete) DeleteListDialog(list, onConfirm = { controller.deleteList(list.id); confirmDelete = false }, onDismiss = { confirmDelete = false })
}

@Composable
private fun NameListDialog(title: String, initial: String, action: String, onDone: (String) -> Unit, onDismiss: () -> Unit) {
    var name by rememberSaveable { mutableStateOf(initial) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { AppTextField(name, { name = it.take(200) }, "Name", keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Sentences)) },
        confirmButton = { TextButton(onClick = { onDone(name.trim()) }, enabled = name.isNotBlank() && name.trim() != initial) { Text(action) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun DeleteListDialog(list: BookList, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Delete \"${list.name}\"?") },
        text = { Text("The list goes away; its books stay in the library and on your shelf.") },
        confirmButton = { TextButton(onClick = onConfirm) { Text("Delete", color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

/** Toggle [book] in and out of each list, or start a new one with it. */
@Composable
internal fun AddToListDialog(controller: AppController, book: Book, onDismiss: () -> Unit) {
    LaunchedEffect(Unit) { controller.loadLists() }
    val lists = controller.listsUi.lists
    var naming by rememberSaveable { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Lists") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(book.title, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                GroupCard {
                    lists.forEachIndexed { index, list ->
                        val member = book.id in list.bookIds
                        AppRow(
                            list.name, "${list.bookCount} ${if (list.bookCount == 1) "book" else "books"}",
                            onClick = { if (member) controller.removeBookFromList(list.id, book.id) else controller.addBookToList(list.id, book) },
                            trailing = { if (member) Icon(BrandIcons.Check, "In this list", tint = MaterialTheme.colorScheme.secondary) },
                        )
                        if (index < lists.lastIndex) RowDivider(inset = 16.dp)
                    }
                    if (lists.isNotEmpty()) RowDivider(inset = 0.dp)
                    AppRow("New list…", null, onClick = { naming = true }, trailing = null)
                }
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text("Done") } },
    )
    if (naming) NameListDialog("New list", "", "Create", onDone = { name ->
        controller.createList(name) { created -> controller.addBookToList(created.id, book) }
        naming = false
    }, onDismiss = { naming = false })
}
