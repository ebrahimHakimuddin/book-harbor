package dev.bookharbor.app.library

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

@Composable
internal fun BrowseFilters(browse: BrowseController, offline: Boolean, series: List<String>) {
    val state = browse.state
    val filter = state.filter
    var selectedId by remember { mutableStateOf<String?>(null) }
    val selected = state.savedFilters.firstOrNull { it.id == selectedId }
    var editing by remember { mutableStateOf(false) }
    var deleting by remember { mutableStateOf(false) }
    var name by remember { mutableStateOf("") }

    Column(Modifier.padding(top = 12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChoice("Library", state.libraries.firstOrNull { it.id == filter.libraryId }?.name ?: if (filter.libraryId.isEmpty()) "All libraries" else "Unavailable library",
                listOf("" to "All libraries") + state.libraries.map { it.id to "${it.name} (${it.bookCount})" }, Modifier.weight(1f)) { browse.setFilter(filter.copy(libraryId = it)) }
            FilterChoice("Format", filter.format.uppercase().ifEmpty { "All formats" }, listOf("" to "All formats", "epub" to "EPUB", "pdf" to "PDF"), Modifier.weight(1f)) { browse.setFilter(filter.copy(format = it)) }
        }
        if (series.isNotEmpty() || filter.series.isNotEmpty()) {
            FilterChoice("Series", filter.series.ifEmpty { "All series" }, listOf("" to "All series") + series.map { it to it }, Modifier.fillMaxWidth()) { browse.setFilter(filter.copy(series = it)) }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            FilterChoice("Saved filter", selected?.name ?: "Saved filters", state.savedFilters.map { it.id to it.name }, Modifier.weight(1f)) { id ->
                state.savedFilters.firstOrNull { it.id == id }?.let { selectedId = id; browse.setFilter(it.filter) }
            }
            TextButton(enabled = !offline && !state.saving, onClick = {
                browse.clearFilterError(); name = selected?.name.orEmpty(); editing = true
            }) { Text("Save") }
        }
        if (filter != CatalogFilter()) Row {
            TextButton(onClick = { selectedId = null; browse.setFilter(CatalogFilter()) }) { Text("Clear filters") }
            if (filter.tag.isNotEmpty()) TextButton(onClick = { browse.setFilter(filter.copy(tag = "")) }) { Text("Remove tag: ${filter.tag}") }
        }
        if (selected != null) Row {
            Text(if (selected.filter == filter) "Saved: ${selected.name}" else "Changed from: ${selected.name}", Modifier.weight(1f).padding(top = 12.dp), style = MaterialTheme.typography.bodySmall)
            TextButton(enabled = !offline && !state.saving, onClick = { deleting = true }) { Text("Delete") }
        }
        if (state.filterError != null && !editing && !deleting) {
            Text(state.filterError, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            if (!offline) TextButton(onClick = browse::loadOptions) { Text("Reload filters and libraries") }
        }
    }

    if (editing) AlertDialog(
        onDismissRequest = { if (!state.saving) editing = false },
        title = { Text(if (selected == null) "Save filter" else "Edit saved filter") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text("Save the current search, library, format, tag, and series for later.")
                OutlinedTextField(name, { name = it.take(100) }, label = { Text("Name") }, singleLine = true, enabled = !state.saving)
                if (state.filterError != null) Text(state.filterError, color = MaterialTheme.colorScheme.error)
            }
        },
        confirmButton = {
            Column {
                if (selected != null) TextButton(enabled = name.isNotBlank() && !state.saving && !offline, onClick = {
                    browse.saveFilter(name, selected.id) { editing = false }
                }) { Text("Update filter") }
                TextButton(enabled = name.isNotBlank() && !state.saving && !offline, onClick = {
                    browse.saveFilter(name) { selectedId = null; editing = false }
                }) { Text(if (state.saving) "Saving…" else "Save new filter") }
            }
        },
        dismissButton = { TextButton(enabled = !state.saving, onClick = { editing = false }) { Text("Cancel") } },
    )
    if (deleting && selected != null) AlertDialog(
        onDismissRequest = { if (!state.saving) deleting = false },
        title = { Text("Delete saved filter?") },
        text = { Column { Text("Delete “${selected.name}”? Your books will stay in the library."); state.filterError?.let { Text(it, color = MaterialTheme.colorScheme.error) } } },
        confirmButton = { TextButton(enabled = !state.saving && !offline, onClick = { browse.deleteFilter(selected.id) { selectedId = null; deleting = false } }) { Text("Delete") } },
        dismissButton = { TextButton(enabled = !state.saving, onClick = { deleting = false }) { Text("Cancel") } },
    )
}

@Composable
private fun FilterChoice(label: String, value: String, options: List<Pair<String, String>>, modifier: Modifier, onSelect: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    Box(modifier) {
        OutlinedButton(onClick = { expanded = true }, enabled = options.isNotEmpty(), modifier = Modifier.fillMaxWidth()) { Text("$label: $value", maxLines = 2) }
        DropdownMenu(expanded, onDismissRequest = { expanded = false }) {
            options.forEach { (id, name) -> DropdownMenuItem(text = { Text(name) }, onClick = { expanded = false; onSelect(id) }) }
        }
    }
}
