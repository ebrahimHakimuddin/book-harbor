package dev.bookharbor.app.library

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
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
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp

/** Ask for a book Hardcover knows about but the library doesn't have, and track your own requests. */
@Composable
fun RequestsTab(controller: AppController) {
    LaunchedEffect(Unit) { controller.loadBookRequests() }
    val state = controller.bookRequestsUi
    var query by rememberSaveable { mutableStateOf("") }

    PullToRefreshBox(isRefreshing = state.loading, onRefresh = controller::loadBookRequests, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(20.dp)) {
        item { Text("Requests", style = MaterialTheme.typography.headlineLarge) }
        item {
            Text(
                "Can't find a book? Search for it and ask for it to be added.",
                Modifier.padding(top = 8.dp, bottom = 14.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        item {
            OutlinedTextField(
                value = query, onValueChange = { query = it }, singleLine = true, modifier = Modifier.fillMaxWidth(),
                label = { Text("Search by title or author") }, shape = RoundedCornerShape(12.dp),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                keyboardActions = KeyboardActions(onSearch = { controller.searchForRequest(query) }),
                trailingIcon = { if (state.searching) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) },
            )
        }
        if (state.error != null) item {
            Text(state.error, Modifier.padding(top = 12.dp), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
        }
        if (state.searchResults.isNotEmpty()) {
            item { SectionLabel("Results") }
            items(state.searchResults, key = { it.provider + it.id }) { candidate ->
                CandidateRow(candidate, onRequest = { controller.requestBook(candidate) })
            }
        }
        item { SectionLabel("Your requests") }
        if (state.mine.isEmpty() && !state.loading) item {
            Text("No requests yet.", Modifier.padding(vertical = 12.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        items(state.mine, key = { it.id }) { request -> BookRequestRow(request, onCancel = { controller.cancelBookRequest(request.id) }) }
    }
    }
}

@Composable
private fun CandidateRow(candidate: MetadataCandidate, onRequest: () -> Unit) {
    var requested by remember(candidate) { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(candidate.title, style = MaterialTheme.typography.titleSmall)
            if (candidate.authors.isNotEmpty()) Text(candidate.authors.joinToString(", "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        TextButton(onClick = { requested = true; onRequest() }, enabled = !requested) { Text(if (requested) "Requested" else "Request") }
    }
}

@Composable
private fun BookRequestRow(request: BookRequest, onCancel: () -> Unit) {
    var confirmCancel by remember { mutableStateOf(false) }
    Row(Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(request.title, style = MaterialTheme.typography.titleSmall)
            val (label, color) = when (request.status) {
                "fulfilled" -> "Added to the library" to MaterialTheme.colorScheme.secondary
                "declined" -> "Declined" to MaterialTheme.colorScheme.error
                else -> "Waiting for review" to MaterialTheme.colorScheme.onSurfaceVariant
            }
            Text(label, style = MaterialTheme.typography.bodySmall, color = color)
        }
        if (request.status == "open") {
            TextButton(onClick = { confirmCancel = true }) { Text("Cancel") }
            if (confirmCancel) {
                AlertDialog(
                    onDismissRequest = { confirmCancel = false },
                    title = { Text("Cancel this request?") },
                    text = { Text("\"${request.title}\" will no longer be waiting for review.") },
                    confirmButton = { TextButton(onClick = { confirmCancel = false; onCancel() }) { Text("Cancel request", color = MaterialTheme.colorScheme.error) } },
                    dismissButton = { TextButton(onClick = { confirmCancel = false }) { Text("Keep it") } },
                )
            }
        }
    }
}
