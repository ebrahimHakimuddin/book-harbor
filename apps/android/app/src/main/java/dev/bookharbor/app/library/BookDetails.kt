package dev.bookharbor.app.library

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.Crossfade
import androidx.compose.animation.animateColorAsState
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.material3.IconButton
import androidx.compose.ui.graphics.graphicsLayer
import dev.bookharbor.app.reader.chapterReadStates
import dev.bookharbor.app.ui.AppRow
import dev.bookharbor.app.ui.AppTextField
import dev.bookharbor.app.ui.GroupCard
import dev.bookharbor.app.ui.Motion
import dev.bookharbor.app.ui.PrimaryButton
import dev.bookharbor.app.ui.RowAction
import dev.bookharbor.app.ui.RowDivider
import dev.bookharbor.app.ui.SecondaryButton
import dev.bookharbor.app.ui.SectionHeader
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

            // Not on the device yet: the main action downloads and the sheet stays open, showing
            // progress, then turns into Start reading. Nothing opens until the reader asks.
            val preferred = preferredEdition(book) { catalog.downloads[it.id] == DownloadStatus.AVAILABLE }
            PrimaryButton(
                when {
                    downloading -> fraction?.let { "Downloading… ${(it * 100).roundToInt()}%" } ?: "Downloading…"
                    downloaded.isEmpty() -> "Download"
                    isReading(progress) -> "Continue reading"
                    isFinished(progress) -> "Read again"
                    else -> "Start reading"
                },
                onClick = {
                    if (downloaded.isEmpty()) { preferred?.let { controller.download(it) }; return@PrimaryButton }
                    onDismiss()
                    // Reading a finished book again starts at the beginning, not on its last page.
                    val again = downloaded.firstOrNull { it.format == "epub" } ?: downloaded.first()
                    if (isFinished(progress)) controller.open(book, again, if (again.format == "epub") Locator.epub(EpubPosition.atChapter(0).toCfi()) else Locator.pdf(1))
                    else controller.open(book)
                },
                modifier = Modifier.padding(top = 20.dp),
                enabled = !downloading,
                icon = if (downloaded.isEmpty() && !downloading) BrandIcons.Download else null,
            )
            Row(Modifier.fillMaxWidth().padding(top = 10.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                SecondaryButton(
                    if (isFinished(progress)) "Mark as unread" else "Mark as read",
                    onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markRead(book, !isFinished(progress)) },
                    modifier = Modifier.weight(1f), icon = BrandIcons.Check,
                )
                SecondaryButton("Add to list", onClick = { addToList = true }, modifier = Modifier.weight(1f), icon = BrandIcons.Bookmark)
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

            JumpToSection(controller, catalog, book, downloaded, onOpened = onDismiss)

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

private enum class ChapterFilter(val label: String) { All("All"), Unread("Unread"), Read("Read") }

/**
 * Chapters of a downloaded EPUB (or a page number for a PDF), to open the book right there.
 * Read chapters are dimmed with a check; the current one is highlighted. Press and hold starts
 * selecting, to mark chapters read or unread, or everything up to one as read.
 */
@Composable
private fun JumpToSection(controller: AppController, catalog: LibraryUiState.Catalog, book: Book, downloaded: List<Edition>, onOpened: () -> Unit) {
    val edition = downloaded.firstOrNull { it.format == "epub" } ?: downloaded.firstOrNull()
    if (edition == null) {
        SectionHeader("Chapters")
        val preferred = preferredEdition(book) { false }
        val downloading = book.editions.any { catalog.downloads[it.id] == DownloadStatus.DOWNLOADING }
        GroupCard {
            AppRow(
                "Download to see chapters", "Then open the book at any chapter, and track which you've read.", icon = BrandIcons.Download,
                trailing = { if (downloading) DownloadRing(book.editions.firstNotNullOfOrNull { catalog.downloadProgress[it.id] }, Modifier.size(22.dp)) else RowAction("Download") },
                onClick = { if (!downloading) preferred?.let { controller.download(it) } },
            )
        }
        return
    }
    val entries by produceState<List<String>?>(null, edition.id) { value = controller.tableOfContents(edition) }
    val haptics = LocalHapticFeedback.current
    val here by produceState<dev.bookharbor.app.sync.LocalPosition?>(null, book.id) { value = kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) { controller.savedPosition(book.id) } }
    val list = entries
    val saved = here
    if (edition.format == "pdf") {
        SectionHeader("Go to page")
        if (list == null) Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp) }
        else PageJump(list.size, current = saved?.locator?.takeIf { it.kind == Locator.PDF }?.page) { page -> onOpened(); controller.open(book, edition, Locator.pdf(page)) }
        return
    }
    if (list == null) {
        SectionHeader("Chapters")
        Box(Modifier.fillMaxWidth().padding(16.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp) }
        return
    }
    val current = saved?.locator?.takeIf { it.kind == Locator.EPUB && saved.editionId == edition.id }?.let { EpubPosition.parse(it.value)?.chapterIndex }
    val version = controller.chapterMarksVersion
    val finishedBook = isFinished(catalog.progress[book.id])
    val states = remember(list.size, current, version, finishedBook) {
        val explicit = controller.chapterMarks(book.id)
        chapterReadStates(list.size, explicit, if (finishedBook) list.size else current)
    }
    var filter by rememberSaveable { mutableStateOf(ChapterFilter.All) }
    var descending by rememberSaveable { mutableStateOf(false) }
    var selected by remember { mutableStateOf(setOf<Int>()) }
    val selecting = selected.isNotEmpty()
    androidx.activity.compose.BackHandler(enabled = selecting) { selected = emptySet() }
    val readCount = states.count { it }
    val visible = remember(states, filter, descending) {
        list.indices.filter { when (filter) { ChapterFilter.All -> true; ChapterFilter.Unread -> !states[it]; ChapterFilter.Read -> states[it] } }
            .let { if (descending) it.reversed() else it }
    }

    // Header: counts and sort, or -- while selecting -- the selection's actions.
    Row(Modifier.fillMaxWidth().padding(top = 28.dp, bottom = 10.dp).heightIn(min = 40.dp), verticalAlignment = Alignment.CenterVertically) {
        Crossfade(selecting, Modifier.weight(1f), label = "chapter header") { isSelecting ->
            if (!isSelecting) Column {
                Text("CHAPTERS", Modifier.semantics { heading() }, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary, letterSpacing = 1.2.sp)
                Text("$readCount of ${list.size} read", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            } else Text("${selected.size} selected", style = MaterialTheme.typography.titleMedium)
        }
        if (!selecting) {
            IconButton(onClick = { descending = !descending }) {
                Icon(BrandIcons.Sort, if (descending) "Sorted last to first. Sort first to last" else "Sorted first to last. Sort last to first", Modifier.size(20.dp).graphicsLayer { scaleY = if (descending) -1f else 1f }, tint = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        } else {
            TextButton(onClick = { selected = emptySet() }) { Text("Cancel") }
        }
    }
    if (!selecting) Row(Modifier.padding(bottom = 10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        ChapterFilter.entries.forEach { option ->
            val count = when (option) { ChapterFilter.All -> list.size; ChapterFilter.Unread -> list.size - readCount; ChapterFilter.Read -> readCount }
            ShelfChip(option.label, count, filter == option) { filter = option }
        }
    } else Row(Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        SecondaryButton("Read", onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markChapters(book, selected, true); selected = emptySet() }, modifier = Modifier.weight(1f), icon = BrandIcons.Check)
        SecondaryButton("Unread", onClick = { controller.markChapters(book, selected, false); selected = emptySet() }, modifier = Modifier.weight(1f))
        if (selected.size == 1) SecondaryButton("All up to here", onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markChaptersReadUpTo(book, selected.first()); selected = emptySet() }, modifier = Modifier.weight(1.4f))
    }

    val state = rememberLazyListState(initialFirstVisibleItemIndex = ((current ?: 0) - 2).coerceAtLeast(0).coerceAtMost((visible.size - 1).coerceAtLeast(0)))
    GroupCard {
        if (visible.isEmpty()) Text(
            if (filter == ChapterFilter.Unread) "Every chapter is read." else "No chapters marked read yet.",
            Modifier.padding(20.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        // Bounded height: the sheet scrolls as a whole, the chapter list scrolls within it.
        LazyColumn(Modifier.fillMaxWidth().heightIn(max = 420.dp), state = state) {
            itemsIndexed(visible, key = { _, index -> index }) { position, index ->
                ChapterRow(
                    number = index + 1, title = list[index], read = states[index], isCurrent = index == current,
                    selecting = selecting, selected = index in selected,
                    onTap = {
                        if (selecting) selected = if (index in selected) selected - index else selected + index
                        else { haptics.performHapticFeedback(HapticFeedbackType.ContextClick); onOpened(); controller.open(book, edition, Locator.epub(EpubPosition.atChapter(index).toCfi())) }
                    },
                    onLongPress = { haptics.performHapticFeedback(HapticFeedbackType.LongPress); selected = selected + index },
                    modifier = Modifier.animateItem(),
                )
                if (position < visible.lastIndex) RowDivider(inset = 60.dp)
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun ChapterRow(number: Int, title: String, read: Boolean, isCurrent: Boolean, selecting: Boolean, selected: Boolean, onTap: () -> Unit, onLongPress: () -> Unit, modifier: Modifier = Modifier) {
    val fade by animateFloatAsState(if (read && !isCurrent) 0.5f else 1f, Motion.standard(), label = "read")
    val tint by animateColorAsState(
        when { selected -> MaterialTheme.colorScheme.secondary.copy(alpha = 0.16f); isCurrent -> MaterialTheme.colorScheme.secondary.copy(alpha = 0.08f); else -> androidx.compose.ui.graphics.Color.Transparent },
        Motion.standard(), label = "row",
    )
    Row(
        modifier.fillMaxWidth().background(tint)
            .combinedClickable(onClickLabel = if (selecting) "Select chapter $number" else "Open at chapter $number", onLongClickLabel = "Select chapters", role = Role.Button, onLongClick = onLongPress, onClick = onTap)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // Leading: the number, or a selection circle while selecting.
        Box(Modifier.width(30.dp), contentAlignment = Alignment.CenterStart) {
            Crossfade(selecting, label = "selection") { isSelecting ->
                if (isSelecting) Box(
                    Modifier.size(22.dp).clip(CircleShape).background(if (selected) MaterialTheme.colorScheme.secondary else androidx.compose.ui.graphics.Color.Transparent)
                        .border(1.5.dp, if (selected) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.5f), CircleShape),
                    contentAlignment = Alignment.Center,
                ) { if (selected) Icon(BrandIcons.Check, null, Modifier.size(14.dp), tint = MaterialTheme.colorScheme.onSecondary) }
                else Text("$number", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary.copy(alpha = fade))
            }
        }
        Column(Modifier.weight(1f).padding(horizontal = 8.dp)) {
            Text(
                title, style = MaterialTheme.typography.bodyLarge, maxLines = 2, overflow = TextOverflow.Ellipsis,
                fontWeight = if (isCurrent) FontWeight.SemiBold else FontWeight.Normal,
                color = MaterialTheme.colorScheme.onSurface.copy(alpha = fade),
            )
            if (isCurrent) Text("You're here", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
        }
        if (read && !selecting) Icon(BrandIcons.Check, "Read", Modifier.size(18.dp), tint = MaterialTheme.colorScheme.secondary.copy(alpha = 0.8f))
    }
}

@Composable
private fun PageJump(pageCount: Int, current: Int?, onGo: (Int) -> Unit) {
    var value by rememberSaveable { mutableStateOf(current?.toString().orEmpty()) }
    val page = value.toIntOrNull()?.takeIf { it in 1..pageCount }
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        AppTextField(
            value, { value = it.filter(Char::isDigit).take(6) }, "Page (1–$pageCount)", Modifier.weight(1f), isError = value.isNotEmpty() && page == null,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number, imeAction = ImeAction.Go),
            keyboardActions = androidx.compose.foundation.text.KeyboardActions(onGo = { page?.let(onGo) }),
        )
        SecondaryButton("Open", onClick = { page?.let(onGo) }, enabled = page != null)
    }
}

/** "1.2 MB", "840 KB". */
fun formatBytes(bytes: Long): String = when {
    bytes <= 0 -> ""
    bytes < 1024 * 1024 -> "${(bytes / 1024.0).roundToInt().coerceAtLeast(1)} KB"
    bytes < 1024L * 1024 * 1024 -> String.format("%.1f MB", bytes / (1024.0 * 1024))
    else -> String.format("%.1f GB", bytes / (1024.0 * 1024 * 1024))
}
