package dev.bookharbor.app.library

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import dev.bookharbor.app.ui.BrandIcons
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/** A reader's own named lists of books: an index of lists, and each list's contents. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ListsScreen(controller: AppController, onClose: () -> Unit) {
    LaunchedEffect(Unit) { controller.loadLists() }
    val state = controller.listsUi

    if (state.openList != null) {
        ListDetailScreen(controller, state.openList, state.booksInOpenList, onBack = controller::closeList)
        return
    }

    var newListName by rememberSaveable { mutableStateOf<String?>(null) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("Your lists") },
                navigationIcon = { IconButton(onClick = onClose) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back to library") } },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().statusBarsPadding()) {
        LazyColumn(Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(20.dp)) {
            if (state.error != null) item {
                Text(state.error, Modifier.padding(bottom = 12.dp), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            }
            item {
                Button(onClick = { newListName = "" }, shape = RoundedCornerShape(9.dp)) { Text("New list") }
            }
            if (state.lists.isEmpty() && !state.loading) item {
                Text(
                    "No lists yet. Make one, then add books to it from a book's \"⋮\" menu.",
                    Modifier.padding(vertical = 24.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            items(state.lists, key = { it.id }) { list ->
                Row(
                    Modifier.fillMaxWidth().padding(vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    TextButton(onClick = { controller.openList(list) }) {
                        Text(list.name, style = MaterialTheme.typography.titleMedium)
                        Text("  ${list.bookCount}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    TextButton(onClick = { controller.deleteList(list.id) }) { Text("Delete", color = MaterialTheme.colorScheme.error) }
                }
            }
        }
        }
    }

    if (newListName != null) {
        var name by rememberSaveable(newListName) { mutableStateOf(newListName.orEmpty()) }
        AlertDialog(
            onDismissRequest = { newListName = null },
            title = { Text("New list") },
            text = { OutlinedTextField(value = name, onValueChange = { name = it }, singleLine = true, label = { Text("Name") }, modifier = Modifier.fillMaxWidth()) },
            confirmButton = { TextButton(onClick = { controller.createList(name.trim()); newListName = null }, enabled = name.isNotBlank()) { Text("Create") } },
            dismissButton = { TextButton(onClick = { newListName = null }) { Text("Cancel") } },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ListDetailScreen(controller: AppController, list: BookList, books: List<Book>, onBack: () -> Unit) {
    val loading = controller.listsUi.loading
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(list.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back to your lists") } },
                actions = { if (books.isNotEmpty()) TextButton(onClick = { controller.downloadAll(books) }) { Icon(BrandIcons.Download, null, Modifier.size(18.dp)); Text("  Download all") } },
            )
        },
    ) { padding ->
        Column(Modifier.fillMaxSize().statusBarsPadding()) {
        LazyColumn(Modifier.fillMaxSize().padding(padding), contentPadding = PaddingValues(20.dp)) {
            if (books.isEmpty() && !loading) item {
                Text("No books in this list yet.", Modifier.padding(vertical = 24.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            items(books, key = { it.id }) { book ->
                Row(Modifier.fillMaxWidth().padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                    Cover(book, controller.covers, Modifier.width(44.dp))
                    Column(Modifier.weight(1f).padding(horizontal = 14.dp)) {
                        Text(book.title, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        val by = book.authors.joinToString(", ")
                        if (by.isNotBlank()) Text(by, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    TextButton(onClick = { controller.removeBookFromList(list.id, book.id) }) { Text("Remove") }
                }
            }
        }
        }
    }
}
