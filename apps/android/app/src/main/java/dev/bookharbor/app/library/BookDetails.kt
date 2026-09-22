package dev.bookharbor.app.library

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.animation.fadeIn
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
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
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.bookharbor.app.reader.epub.EpubPosition
import dev.bookharbor.app.sync.Locator
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.theme.cautionColor
import kotlin.math.roundToInt

/**
 * Everything about one book: its description, series and tags, reading progress, and the
 * actions on it -- including opening straight at a chosen chapter or page.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BookDetailsSheet(controller: AppController, catalog: LibraryUiState.Catalog, book: Book, onDismiss: () -> Unit) {
    val progress = catalog.progress[book.id]
    val downloaded = book.editions.filter { catalog.downloads[it.id] == DownloadStatus.AVAILABLE }
    val downloading = book.editions.any { catalog.downloads[it.id] == DownloadStatus.DOWNLOADING }
    val fraction = book.editions.firstNotNullOfOrNull { catalog.downloadProgress[it.id] }
    val haptics = LocalHapticFeedback.current
    var addToList by remember { mutableStateOf(false) }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = MaterialTheme.colorScheme.surface) {
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).padding(bottom = 32.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                Cover(book, controller.covers, Modifier.width(104.dp).shadow(10.dp, RoundedCornerShape(6.dp)))
                Column(Modifier.weight(1f).padding(start = 18.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    seriesLabel(book).takeIf { it.isNotBlank() }?.let {
                        Text(it.uppercase(), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary, letterSpacing = 1.2.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    Text(book.title, Modifier.semantics { heading() }, style = MaterialTheme.typography.headlineSmall, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    if (book.subtitle.isNotBlank()) Text(book.subtitle, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    if (book.authors.isNotEmpty()) Text(book.authors.joinToString(", "), style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    Text(
                        book.editions.joinToString(" · ") { "${it.format.uppercase()} ${formatBytes(it.byteLength)}" },
                        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            if (progress != null && progress > 0) {
                val shown by animateFloatAsState(progress.toFloat().coerceIn(0f, 1f), tween(500), label = "progress")
                Row(Modifier.padding(top = 20.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.weight(1f).height(6.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant)) {
                        Box(Modifier.fillMaxWidth(shown).fillMaxHeight().clip(CircleShape).background(MaterialTheme.colorScheme.secondary))
                    }
                    Text(if (isFinished(progress)) "Finished" else "${(progress * 100).roundToInt()}% read", Modifier.padding(start = 12.dp), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary)
                }
            }

            Button(
                onClick = {
                    onDismiss()
                    // Reading a finished book again starts at the beginning, not on its last page.
                    val again = downloaded.firstOrNull { it.format == "epub" } ?: downloaded.firstOrNull()
                    if (isFinished(progress) && again != null) controller.open(book, again, if (again.format == "epub") Locator.epub(EpubPosition.atChapter(0).toCfi()) else Locator.pdf(1))
                    else controller.open(book)
                },
                enabled = !downloading,
                shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(top = 20.dp).height(52.dp),
            ) {
                when {
                    downloading -> { DownloadRing(fraction, Modifier.size(18.dp), color = MaterialTheme.colorScheme.onPrimary); Text(fraction?.let { "  Downloading… ${(it * 100).roundToInt()}%" } ?: "  Downloading…") }
                    downloaded.isEmpty() -> { Icon(BrandIcons.Download, null, Modifier.size(18.dp)); Text("  Download and read") }
                    isReading(progress) -> Text("Continue reading")
                    isFinished(progress) -> Text("Read again")
                    else -> Text("Start reading")
                }
            }
            Row(Modifier.fillMaxWidth().padding(top = 10.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedButton(onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markRead(book, !isFinished(progress)) }, Modifier.weight(1f), shape = RoundedCornerShape(12.dp)) {
                    Icon(BrandIcons.Check, null, Modifier.size(16.dp)); Text(if (isFinished(progress)) "  Mark unread" else "  Mark read", maxLines = 1)
                }
                OutlinedButton(onClick = { addToList = true }, Modifier.weight(1f), shape = RoundedCornerShape(12.dp)) {
                    Icon(BrandIcons.Request, null, Modifier.size(16.dp)); Text("  Add to list", maxLines = 1)
                }
            }

            if (book.tags.isNotEmpty()) {
                Row(Modifier.padding(top = 20.dp).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    book.tags.forEach { tag ->
                        Text(tag, Modifier.clip(CircleShape).background(MaterialTheme.colorScheme.surfaceVariant).padding(horizontal = 12.dp, vertical = 6.dp), style = MaterialTheme.typography.labelMedium)
                    }
                }
            }

            if (book.description.isNotBlank()) {
                var expanded by rememberSaveable { mutableStateOf(false) }
                Text("About this book", Modifier.padding(top = 24.dp, bottom = 8.dp).semantics { heading() }, style = MaterialTheme.typography.titleMedium)
                Text(
                    book.description,
                    Modifier.animateContentSize().clickable(onClickLabel = if (expanded) "Show less" else "Show more", role = Role.Button) { expanded = !expanded },
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = if (expanded) Int.MAX_VALUE else 5, overflow = TextOverflow.Ellipsis,
                )
                if (book.description.length > 280) TextButton(onClick = { expanded = !expanded }) { Text(if (expanded) "Show less" else "Show more") }
            }

            JumpToSection(controller, book, downloaded, onOpened = onDismiss)

            if (downloaded.isNotEmpty()) {
                var confirm by remember { mutableStateOf<Edition?>(null) }
                downloaded.forEach { edition ->
                    TextButton(onClick = { confirm = edition }, Modifier.padding(top = 12.dp)) {
                        Icon(BrandIcons.Trash, null, Modifier.size(16.dp), tint = cautionColor())
                        Text("  Remove ${edition.format.uppercase()} download", color = cautionColor())
                    }
                }
                confirm?.let { edition ->
                    androidx.compose.material3.AlertDialog(
                        onDismissRequest = { confirm = null },
                        title = { Text("Remove download?") },
                        text = { Text("\"${book.title}\" (${edition.format.uppercase()}) will be deleted from this device. You can download it again any time.") },
                        confirmButton = { TextButton(onClick = { controller.removeDownload(edition); confirm = null }) { Text("Remove", color = cautionColor()) } },
                        dismissButton = { TextButton(onClick = { confirm = null }) { Text("Cancel") } },
                    )
                }
            }
        }
    }
    if (addToList) AddToListDialog(controller, book, onDismiss = { addToList = false })
}

/** Chapters of a downloaded EPUB, or a page number for a PDF: open the book right there. */
@Composable
private fun JumpToSection(controller: AppController, book: Book, downloaded: List<Edition>, onOpened: () -> Unit) {
    val edition = downloaded.firstOrNull { it.format == "epub" } ?: downloaded.firstOrNull()
    Text(if (edition?.format == "pdf") "Go to page" else "Chapters", Modifier.padding(top = 24.dp, bottom = 4.dp).semantics { heading() }, style = MaterialTheme.typography.titleMedium)
    if (edition == null) {
        Text("Download this book to open it at any chapter.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        return
    }
    val entries by produceState<List<String>?>(null, edition.id) { value = controller.tableOfContents(edition) }
    val haptics = LocalHapticFeedback.current
    val here by produceState<dev.bookharbor.app.sync.LocalPosition?>(null, book.id) { value = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) { controller.savedPosition(book.id) } }
    val list = entries
    val saved = here
    when {
        list == null -> Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp) }
        edition.format == "pdf" -> PageJump(list.size, current = saved?.locator?.takeIf { it.kind == Locator.PDF }?.page) { page ->
            onOpened(); controller.open(book, edition, Locator.pdf(page))
        }
        else -> {
            val current = saved?.locator?.takeIf { it.kind == Locator.EPUB && saved.editionId == edition.id }?.let { EpubPosition.parse(it.value)?.chapterIndex }
            val state = rememberLazyListState(initialFirstVisibleItemIndex = ((current ?: 0) - 2).coerceAtLeast(0))
            AnimatedVisibility(visible = true, enter = fadeIn()) {
                // Bounded height: the sheet scrolls as a whole, the chapter list scrolls within it.
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 360.dp), state = state) {
                    itemsIndexed(list) { index, title ->
                        val isCurrent = index == current
                        Row(
                            Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp))
                                .background(if (isCurrent) MaterialTheme.colorScheme.secondary.copy(alpha = 0.10f) else androidx.compose.ui.graphics.Color.Transparent)
                                .clickable(onClickLabel = "Open at $title", role = Role.Button) { haptics.performHapticFeedback(HapticFeedbackType.ContextClick); onOpened(); controller.open(book, edition, Locator.epub(EpubPosition.atChapter(index).toCfi())) }
                                .padding(horizontal = 12.dp, vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text("${index + 1}", Modifier.width(32.dp), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary)
                            Text(title, Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge, fontWeight = if (isCurrent) FontWeight.SemiBold else FontWeight.Normal, maxLines = 2, overflow = TextOverflow.Ellipsis)
                            if (isCurrent) Text("You're here", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun PageJump(pageCount: Int, current: Int?, onGo: (Int) -> Unit) {
    var value by rememberSaveable { mutableStateOf(current?.toString().orEmpty()) }
    val page = value.toIntOrNull()?.takeIf { it in 1..pageCount }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        OutlinedTextField(
            value, { value = it.filter(Char::isDigit).take(6) }, Modifier.weight(1f), singleLine = true,
            label = { Text("Page (1–$pageCount)") }, isError = value.isNotEmpty() && page == null, shape = RoundedCornerShape(12.dp),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Go),
            keyboardActions = androidx.compose.foundation.text.KeyboardActions(onGo = { page?.let(onGo) }),
        )
        Button(onClick = { page?.let(onGo) }, enabled = page != null, shape = RoundedCornerShape(12.dp), modifier = Modifier.height(56.dp)) { Text("Open") }
    }
}

/** "1.2 MB", "840 KB". */
fun formatBytes(bytes: Long): String = when {
    bytes <= 0 -> ""
    bytes < 1024 * 1024 -> "${(bytes / 1024.0).roundToInt().coerceAtLeast(1)} KB"
    bytes < 1024L * 1024 * 1024 -> String.format("%.1f MB", bytes / (1024.0 * 1024))
    else -> String.format("%.1f GB", bytes / (1024.0 * 1024 * 1024))
}
