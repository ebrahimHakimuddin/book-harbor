package dev.bookharbor.app.library

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

enum class DownloadStatus { NOT_DOWNLOADED, DOWNLOADING, AVAILABLE, FAILED }
sealed interface LibraryUiState { data object Setup : LibraryUiState; data class SignIn(val instance: InstanceInfo? = null, val serverUrl: String = "") : LibraryUiState; data object Loading : LibraryUiState; data class Catalog(val instance: InstanceInfo, val books: List<Book>, val downloads: Map<String, DownloadStatus> = emptyMap(), val errors: Map<String, String> = emptyMap()) : LibraryUiState; data class Error(val message: String, val retry: LibraryUiState) : LibraryUiState }

@Composable
fun LibraryScreen(state: LibraryUiState, onServer: (String) -> Unit, onSignIn: (String, String) -> Unit, onRetry: () -> Unit, onSignOut: () -> Unit, onDownload: (Edition) -> Unit = {}, onRemove: (Edition) -> Unit = {}) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        when (state) {
            LibraryUiState.Setup -> { Text("BookHarbor", style = MaterialTheme.typography.headlineLarge); Text("Connect to your BookHarbor server to begin."); ServerForm("Server URL", "", onServer) }
            is LibraryUiState.SignIn -> { Text(state.instance?.name ?: "BookHarbor", style = MaterialTheme.typography.headlineLarge); Text("Sign in to browse your library."); ServerForm("Server URL", state.serverUrl, onServer); SignInForm(onSignIn) }
            LibraryUiState.Loading -> { Text("Loading your library…"); CircularProgressIndicator() }
            is LibraryUiState.Error -> { Text("Something went wrong", style = MaterialTheme.typography.headlineSmall); Text(state.message); Button(onClick = onRetry) { Text("Try again") } }
            is LibraryUiState.Catalog -> { Text(state.instance.name, style = MaterialTheme.typography.headlineLarge); Text("Library", style = MaterialTheme.typography.titleLarge); if (state.books.isEmpty()) Text("No books yet.") else LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(12.dp)) { items(state.books) { book -> Column(Modifier.fillMaxWidth()) { Text(book.title, style = MaterialTheme.typography.titleMedium); Text("${book.editions.size} edition${if (book.editions.size == 1) "" else "s"}", style = MaterialTheme.typography.bodySmall); book.editions.forEach { edition -> val status = state.downloads[edition.id] ?: DownloadStatus.NOT_DOWNLOADED; androidx.compose.foundation.layout.Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) { Column(Modifier.weight(1f).padding(top = 8.dp)) { Text(edition.originalFilename.ifBlank { edition.format.ifBlank { "Edition" } }, style = MaterialTheme.typography.bodyMedium); Text(when (status) { DownloadStatus.AVAILABLE -> "Available offline"; DownloadStatus.DOWNLOADING -> "Downloading…"; DownloadStatus.FAILED -> state.errors[edition.id] ?: "Download failed — try again"; DownloadStatus.NOT_DOWNLOADED -> "Not downloaded" }, color = if (status == DownloadStatus.FAILED) MaterialTheme.colorScheme.tertiary else MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.labelMedium) }; when (status) { DownloadStatus.AVAILABLE -> TextButton(onClick = { onRemove(edition) }) { Text("Remove") }; DownloadStatus.DOWNLOADING -> { CircularProgressIndicator(Modifier.padding(12.dp)) }; DownloadStatus.NOT_DOWNLOADED, DownloadStatus.FAILED -> Button(onClick = { onDownload(edition) }) { Text(if (status == DownloadStatus.FAILED) "Retry" else "Download") } } } }; HorizontalDivider(Modifier.padding(top = 10.dp)) } } }; Button(onClick = onSignOut) { Text("Sign out") } }
        }
    }
}

@Composable private fun ServerForm(label: String, initial: String, onSubmit: (String) -> Unit) { var value = androidx.compose.runtime.remember(initial) { androidx.compose.runtime.mutableStateOf(initial) }; OutlinedTextField(value.value, { value.value = it }, label = { Text(label) }, modifier = Modifier.fillMaxWidth()); Button(onClick = { onSubmit(value.value) }, enabled = value.value.isNotBlank()) { Text("Connect") } }
@Composable private fun SignInForm(onSignIn: (String, String) -> Unit) { var email = androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf("") }; var password = androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf("") }; OutlinedTextField(email.value, { email.value = it }, label = { Text("Email") }, modifier = Modifier.fillMaxWidth()); OutlinedTextField(password.value, { password.value = it }, visualTransformation = PasswordVisualTransformation(), label = { Text("Password") }, modifier = Modifier.fillMaxWidth()); Button(onClick = { onSignIn(email.value, password.value) }, enabled = email.value.isNotBlank() && password.value.isNotBlank()) { Text("Sign in") } }
