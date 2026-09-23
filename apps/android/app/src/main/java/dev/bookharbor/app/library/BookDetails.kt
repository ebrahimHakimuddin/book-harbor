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
import dev.bookharbor.app.ui.TonalIconAction
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
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.statusBarsPadding
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
 * Everything about one book, as its own page: description, series and tags, reading progress,
 * and the actions on it -- including opening straight at a chosen chapter or page.
 */
@Composable
fun BookDetailsPage(controller: AppController, catalog: LibraryUiState.Catalog, book: Book, onDismiss: () -> Unit) {
    androidx.activity.compose.BackHandler(onBack = onDismiss)
    val progress = catalog.progress[book.id]
    val downloaded = book.editions.filter { catalog.downloads[it.id] == DownloadStatus.AVAILABLE }
    val downloading = book.editions.any { catalog.downloads[it.id] == DownloadStatus.DOWNLOADING }
    val fraction = book.editions.firstNotNullOfOrNull { catalog.downloadProgress[it.id] }
    val haptics = LocalHapticFeedback.current
    var addToList by remember { mutableStateOf(false) }
    androidx.compose.material3.Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(Modifier.fillMaxSize().statusBarsPadding().verticalScroll(rememberScrollState()).navigationBarsPadding().padding(horizontal = 24.dp).padding(bottom = 32.dp)) {
            IconButton(onClick = onDismiss, modifier = Modifier.padding(top = 8.dp, bottom = 8.dp).offset(x = (-12).dp)) {
                Icon(androidx.compose.material.icons.Icons.AutoMirrored.Filled.ArrowBack, "Back")
            }
            Row(verticalAlignment = Alignment.Top) {
                Cover(book, controller.covers, Modifier.width(116.dp).shadow(12.dp, RoundedCornerShape(6.dp)))
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
            // The main action full width, then the book's other actions below it as named icons
            // (press and hold for the name), sharing the row equally.
            var confirmRemove by remember { mutableStateOf<Edition?>(null) }
            Column(Modifier.fillMaxWidth().padding(top = 20.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                PrimaryButton(
                    when {
                        downloading -> fraction?.let { "Downloading… ${(it * 100).roundToInt()}%" } ?: "Downloading…"
                        downloaded.isEmpty() -> "Download"
                        isReading(progress) -> "Continue"
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
                    modifier = Modifier.fillMaxWidth(),
                    enabled = !downloading,
                    icon = if (downloaded.isEmpty() && !downloading) BrandIcons.Download else BrandIcons.Play.takeIf { !downloading },
                )
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    TonalIconAction(
                        BrandIcons.Check, if (isFinished(progress)) "Mark as unread" else "Mark as read",
                        onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markRead(book, !isFinished(progress)) },
                        modifier = Modifier.weight(1f),
                        active = isFinished(progress),
                    )
                    TonalIconAction(BrandIcons.Bookmark, "Add to list", onClick = { addToList = true }, modifier = Modifier.weight(1f))
                    downloaded.firstOrNull()?.let { edition ->
                        TonalIconAction(BrandIcons.Trash, "Remove download", onClick = { confirmRemove = edition }, modifier = Modifier.weight(1f), tint = cautionColor())
                    }
                }
            }
            confirmRemove?.let { edition ->
                androidx.compose.material3.AlertDialog(
                    onDismissRequest = { confirmRemove = null },
                    title = { Text("Remove download?") },
                    text = { Text("\"${book.title}\" will be deleted from this device. Your progress stays, and you can download it again any time.") },
                    confirmButton = { TextButton(onClick = { downloaded.forEach(controller::removeDownload); confirmRemove = null }) { Text("Remove", color = cautionColor()) } },
                    dismissButton = { TextButton(onClick = { confirmRemove = null }) { Text("Cancel") } },
                )
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
        val downloading = book.editions.any { catalog.downloads[it.id] == DownloadStatus.DOWNLOADING }
        // The Download button above is the action; this just says what downloading unlocks.
        Text(
            if (downloading) "Chapters appear here once the download finishes." else "Download the book to open it at any chapter and track which ones you've read.",
            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        return
    }
    val entries by produceState<List<String>?>(null, edition.id) { value = controller.tableOfContents(book, edition) }
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
    val readCount = states.count { it }
    val open: (Int) -> Unit = { index ->
        haptics.performHapticFeedback(HapticFeedbackType.ContextClick)
        onOpened()
        controller.open(book, edition, Locator.epub(EpubPosition.atChapter(index).toCfi()))
    }
    var showAll by rememberSaveable { mutableStateOf(false) }

    // A short preview around where the reader is; every chapter -- search, filters, sort, and
    // marking -- lives on its own page, so a 300-chapter book is as easy as a 5-chapter one.
    Row(Modifier.fillMaxWidth().padding(top = 28.dp, bottom = 10.dp), verticalAlignment = Alignment.Bottom) {
        Column(Modifier.weight(1f)) {
            Text("CHAPTERS", Modifier.semantics { heading() }, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary, letterSpacing = 1.2.sp)
            Text("$readCount of ${list.size} read", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
    val preview = remember(list.size, current) { previewWindow(list.size, current ?: 0, PREVIEW_SIZE) }
    GroupCard {
        preview.forEach { index ->
            ChapterRow(index + 1, list[index], states[index], index == current, selecting = false, selected = false, onTap = { open(index) }, onLongPress = { showAll = true })
            RowDivider(inset = 60.dp)
        }
        AppRow(
            if (list.size > preview.size) "All ${list.size} chapters" else "Manage chapters",
            "Search, filter, and mark chapters read", icon = BrandIcons.List, onClick = { showAll = true },
        )
    }
    if (showAll) ChaptersPage(book.title, list, states, current, controller, book, onOpen = { index -> showAll = false; open(index) }, onBack = { showAll = false })
}

private const val PREVIEW_SIZE = 5

/** Up to [size] chapter indices starting just before [current], shifted to stay within the book. */
internal fun previewWindow(count: Int, current: Int, size: Int): List<Int> {
    if (count <= size) return (0 until count).toList()
    val start = (current - 1).coerceIn(0, count - size)
    return (start until start + size).toList()
}

/**
 * Every chapter on a page of its own: search by title, read/unread filter, sort, and multi-select
 * with a fixed action bar. Opens scrolled to the reader's current chapter.
 */
@Composable
private fun ChaptersPage(
    bookTitle: String,
    titles: List<String>,
    states: List<Boolean>,
    current: Int?,
    controller: AppController,
    book: Book,
    onOpen: (Int) -> Unit,
    onBack: () -> Unit,
) {
    val haptics = LocalHapticFeedback.current
    var query by rememberSaveable { mutableStateOf("") }
    var filter by rememberSaveable { mutableStateOf(ChapterFilter.All) }
    var descending by rememberSaveable { mutableStateOf(false) }
    var selected by remember { mutableStateOf(setOf<Int>()) }
    val selecting = selected.isNotEmpty()
    androidx.activity.compose.BackHandler { if (selecting) selected = emptySet() else onBack() }
    val readCount = states.count { it }
    val visible = remember(states, filter, descending, query) {
        val needle = query.trim().lowercase()
        titles.indices.filter { index ->
            val matchesFilter = when (filter) { ChapterFilter.All -> true; ChapterFilter.Unread -> !states[index]; ChapterFilter.Read -> states[index] }
            matchesFilter && (needle.isEmpty() || titles[index].lowercase().contains(needle) || "${index + 1}" == needle)
        }.let { if (descending) it.reversed() else it }
    }
    val listState = rememberLazyListState(initialFirstVisibleItemIndex = visible.indexOf(current ?: 0).coerceAtLeast(0))

    androidx.compose.ui.window.Dialog(onDismissRequest = onBack, properties = androidx.compose.ui.window.DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        androidx.compose.material3.Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
            Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding()) {
                Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = { if (selecting) selected = emptySet() else onBack() }) {
                        Icon(if (selecting) BrandIcons.Close else androidx.compose.material.icons.Icons.AutoMirrored.Filled.ArrowBack, if (selecting) "Cancel selection" else "Back")
                    }
                    Column(Modifier.weight(1f)) {
                        Text(if (selecting) "${selected.size} selected" else "Chapters", style = MaterialTheme.typography.titleLarge)
                        if (!selecting) Text("$bookTitle · $readCount of ${titles.size} read", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                    if (selecting) TextButton(onClick = { selected = visible.toSet() }) { Text("Select all") }
                    else IconButton(onClick = { descending = !descending }) {
                        Icon(BrandIcons.Sort, if (descending) "Sorted last to first. Sort first to last" else "Sorted first to last. Sort last to first", Modifier.size(20.dp).graphicsLayer { scaleY = if (descending) -1f else 1f })
                    }
                }
                Column(Modifier.padding(horizontal = 20.dp)) {
                    dev.bookharbor.app.ui.SearchPill(query, { query = it }, "Find a chapter by name or number", onSearch = {})
                    Row(Modifier.padding(vertical = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        ChapterFilter.entries.forEach { option ->
                            val count = when (option) { ChapterFilter.All -> titles.size; ChapterFilter.Unread -> titles.size - readCount; ChapterFilter.Read -> readCount }
                            ShelfChip(option.label, count, filter == option) { filter = option }
                        }
                    }
                }
                LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = listState, contentPadding = androidx.compose.foundation.layout.PaddingValues(start = 20.dp, end = 20.dp, bottom = 16.dp)) {
                    if (visible.isEmpty()) item {
                        Text(
                            when { query.isNotBlank() -> "No chapter matches \"$query\"."; filter == ChapterFilter.Unread -> "Every chapter is read."; else -> "No chapters marked read yet." },
                            Modifier.padding(vertical = 24.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                    itemsIndexed(visible, key = { _, index -> index }) { position, index ->
                        val shape = when {
                            visible.size == 1 -> RoundedCornerShape(20.dp)
                            position == 0 -> RoundedCornerShape(topStart = 20.dp, topEnd = 20.dp)
                            position == visible.lastIndex -> RoundedCornerShape(bottomStart = 20.dp, bottomEnd = 20.dp)
                            else -> RoundedCornerShape(0.dp)
                        }
                        Column(Modifier.animateItem().clip(shape).background(MaterialTheme.colorScheme.surfaceVariant)) {
                            ChapterRow(
                                number = index + 1, title = titles[index], read = states[index], isCurrent = index == current,
                                selecting = selecting, selected = index in selected,
                                onTap = { if (selecting) selected = if (index in selected) selected - index else selected + index else onOpen(index) },
                                onLongPress = { haptics.performHapticFeedback(HapticFeedbackType.LongPress); selected = selected + index },
                            )
                            if (position < visible.lastIndex) RowDivider(inset = 60.dp)
                        }
                    }
                }
                // The selection's actions stay put at the bottom, however far the list scrolls.
                AnimatedVisibility(selecting) {
                    Column(Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surface).padding(horizontal = 20.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            SecondaryButton("Mark read", onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markChapters(book, selected, true); selected = emptySet() }, modifier = Modifier.weight(1f))
                            SecondaryButton("Mark unread", onClick = { controller.markChapters(book, selected, false); selected = emptySet() }, modifier = Modifier.weight(1f))
                        }
                        if (selected.size == 1) {
                            val upTo = selected.first()
                            SecondaryButton(
                                if (upTo == 0) "Mark chapter 1 as read" else "Mark chapters 1–${upTo + 1} as read",
                                onClick = { haptics.performHapticFeedback(HapticFeedbackType.Confirm); controller.markChaptersReadUpTo(book, upTo); selected = emptySet() },
                                modifier = Modifier.fillMaxWidth(), icon = BrandIcons.Check,
                            )
                        }
                    }
                }
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
