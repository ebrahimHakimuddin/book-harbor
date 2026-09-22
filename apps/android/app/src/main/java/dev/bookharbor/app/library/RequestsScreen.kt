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
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.ui.draw.clip
import dev.bookharbor.app.ui.AppRow
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.EmptyState
import dev.bookharbor.app.ui.GroupCard
import dev.bookharbor.app.ui.RowDivider
import dev.bookharbor.app.ui.ScreenTitle
import dev.bookharbor.app.ui.SearchPill
import dev.bookharbor.app.ui.SectionHeader

/** Ask for a book the library doesn't have yet, and follow what happens to your requests. */
@Composable
fun RequestsTab(controller: AppController) {
    LaunchedEffect(Unit) { controller.loadBookRequests() }
    val state = controller.bookRequestsUi
    var query by rememberSaveable { mutableStateOf("") }
    val open = state.mine.count { it.status == "open" }

    PullToRefreshBox(isRefreshing = state.loading, onRefresh = controller::loadBookRequests, modifier = Modifier.fillMaxSize().statusBarsPadding()) {
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 32.dp)) {
            item { ScreenTitle("Requests", if (open > 0) "$open waiting for review" else "Ask for a book to be added") }
            item {
                Box(Modifier.padding(top = 14.dp)) {
                    SearchPill(query, { query = it }, "Search by title or author", onSearch = { controller.searchForRequest(query) }, busy = state.searching)
                }
            }
            if (state.error != null) item {
                Text(state.error, Modifier.padding(top = 12.dp), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            }
            if (state.searchResults.isNotEmpty()) {
                item { SectionHeader("Results") }
                item {
                    GroupCard {
                        state.searchResults.forEachIndexed { index, candidate ->
                            CandidateRow(controller, candidate, onRequest = { controller.requestBook(candidate) })
                            if (index < state.searchResults.lastIndex) RowDivider(inset = 70.dp)
                        }
                    }
                }
            }
            item { SectionHeader("Your requests") }
            if (state.mine.isEmpty() && !state.loading) item {
                EmptyState(BrandIcons.Request, "No requests yet", "Search above for a book that isn't in the library, and ask for it. You'll see here when it's added.")
            } else item {
                GroupCard {
                    state.mine.forEachIndexed { index, request ->
                        BookRequestRow(controller, request, onCancel = { controller.cancelBookRequest(request.id) })
                        if (index < state.mine.lastIndex) RowDivider(inset = 70.dp)
                    }
                }
            }
        }
    }
}

@Composable
private fun CandidateRow(controller: AppController, candidate: MetadataCandidate, onRequest: () -> Unit) {
    var requested by remember(candidate) { mutableStateOf(false) }
    AppRow(
        candidate.title, candidate.authors.joinToString(", ").ifBlank { null },
        leading = { Cover(Book(candidate.provider + candidate.id, candidate.title, emptyList(), coverUrl = candidate.coverUrl), controller.covers, Modifier.width(40.dp)) },
        trailing = {
            TextButton(onClick = { requested = true; onRequest() }, enabled = !requested) { Text(if (requested) "Requested" else "Request") }
        },
    )
}

@Composable
private fun BookRequestRow(controller: AppController, request: BookRequest, onCancel: () -> Unit) {
    var confirmCancel by remember { mutableStateOf(false) }
    AppRow(
        request.title, request.author.ifBlank { null },
        leading = { Cover(Book(request.id, request.title, emptyList(), coverUrl = request.coverUrl), controller.covers, Modifier.width(40.dp)) },
        trailing = {
            Column(horizontalAlignment = Alignment.End) {
                StatusPill(request.status)
                if (request.status == "open") TextButton(onClick = { confirmCancel = true }) { Text("Cancel", color = MaterialTheme.colorScheme.onSurfaceVariant) }
            }
        },
    )
    if (confirmCancel) AlertDialog(
        onDismissRequest = { confirmCancel = false },
        title = { Text("Cancel this request?") },
        text = { Text("\"${request.title}\" will no longer be waiting for review.") },
        confirmButton = { TextButton(onClick = { confirmCancel = false; onCancel() }) { Text("Cancel request", color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onClick = { confirmCancel = false }) { Text("Keep it") } },
    )
}

/** A request's status as a small colored pill: waiting, added, or declined. */
@Composable
private fun StatusPill(status: String) {
    val (label, color) = when (status) {
        "fulfilled" -> "Added" to MaterialTheme.colorScheme.secondary
        "declined" -> "Declined" to MaterialTheme.colorScheme.error
        else -> "Waiting" to MaterialTheme.colorScheme.onSurfaceVariant
    }
    Text(label, Modifier.clip(CircleShape).background(color.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp), style = MaterialTheme.typography.labelMedium, color = color)
}
