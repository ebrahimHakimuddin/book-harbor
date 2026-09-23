package dev.bookharbor.app.library

import android.content.Intent
import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Switch
import androidx.core.content.ContextCompat
import android.net.Uri
import androidx.compose.foundation.BorderStroke
import androidx.compose.animation.animateContentSize
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.border
import androidx.compose.foundation.selection.selectable
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.layout
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.Crossfade
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.material3.ripple
import androidx.compose.ui.composed
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import dev.bookharbor.app.ui.pressScale
import androidx.compose.material3.PlainTooltip
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
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
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.offset
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
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
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
import androidx.compose.ui.text.input.KeyboardCapitalization
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

/**
 * What a book's card shows, derived once per book from the catalog. Cards take this instead of the
 * whole catalog, so a download ticking along redraws only its own card, not every one on screen.
 */
@androidx.compose.runtime.Immutable
internal data class BookCardState(
    val progress: Double?,
    val downloaded: Boolean,
    val downloading: Boolean,
    val fraction: Float?,
    val error: String?,
    val chaptersLeft: Int?,
    val onShelf: Boolean,
)

/** [downloaded] is the set of downloaded edition IDs, computed once per screen rather than per card. */
internal fun LibraryUiState.Catalog.cardState(book: Book, downloaded: Set<String>): BookCardState {
    val progress = progress[book.id]
    val isDownloaded = book.editions.any { it.id in downloaded }
    return BookCardState(
        progress = progress,
        downloaded = isDownloaded,
        downloading = book.editions.any { downloads[it.id] == DownloadStatus.DOWNLOADING },
        fraction = book.editions.firstNotNullOfOrNull { downloadProgress[it.id] },
        error = book.editions.firstNotNullOfOrNull { edition -> errors[edition.id]?.takeIf { downloads[edition.id] == DownloadStatus.FAILED } },
        chaptersLeft = chaptersLeft[book.id],
        onShelf = (progress ?: 0.0) > 0.0 || isDownloaded,
    )
}

internal fun LibraryUiState.Catalog.downloadedEditions(): Set<String> = downloads.filterValues { it == DownloadStatus.AVAILABLE }.keys

/** One short line of where the reader is: chapters left when known, else percent or download state. */
internal fun cardStatus(state: BookCardState, book: Book): String = when {
    state.downloading -> state.fraction?.let { "Downloading… ${(it * 100).roundToInt()}%" } ?: "Downloading…"
    state.error != null -> "Download failed"
    isFinished(state.progress) -> "Finished"
    isReading(state.progress) -> state.chaptersLeft?.let { chaptersLeftLabel(it) } ?: "${((state.progress ?: 0.0) * 100).roundToInt()}% read"
    state.downloaded -> state.chaptersLeft?.let { "$it ${if (it == 1) "chapter" else "chapters"}" } ?: "Downloaded"
    else -> book.authors.firstOrNull() ?: "Not downloaded"
}

internal fun chaptersLeftLabel(left: Int) = when (left) { 0 -> "Last pages"; 1 -> "1 chapter left"; else -> "$left chapters left" }

@Composable
internal fun BookRow(controller: AppController, book: Book, state: BookCardState) {
    var menu by remember { mutableStateOf(false) }
    Column {
        Row(
            Modifier.fillMaxWidth().bookClicks(book, scale = false, onTap = { controller.showDetails(book) }, onLongPress = { menu = true }).padding(vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(Modifier.width(52.dp)) {
                Cover(book, controller.covers, Modifier.fillMaxWidth())
                if (state.downloaded && !state.downloading) ResumeButton(book, isReading(state.progress), Modifier.align(Alignment.BottomEnd).padding(3.dp), small = true) { controller.open(book) }
            }
            Column(Modifier.weight(1f).padding(horizontal = 14.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(book.title, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onBackground)
                val by = book.authors.joinToString(", ").ifBlank { book.subtitle }
                if (by.isNotBlank()) Text(by, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                seriesLabel(book).takeIf { it.isNotBlank() }?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis) }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (state.downloading) DownloadRing(state.fraction, Modifier.padding(end = 6.dp).size(12.dp))
                    Text(
                        cardStatus(state, book), style = MaterialTheme.typography.labelMedium, maxLines = 1,
                        color = when { state.error != null -> MaterialTheme.colorScheme.error; isReading(state.progress) || state.downloading -> MaterialTheme.colorScheme.secondary; else -> MaterialTheme.colorScheme.onSurfaceVariant },
                    )
                }
            }
            BookMenu(controller, book, menu, { menu = it }, onDetails = { controller.showDetails(book) })
        }
        HorizontalDivider(Modifier.padding(start = 66.dp), color = MaterialTheme.colorScheme.outline.copy(alpha = 0.2f))
    }
}

@Composable
internal fun BookGridCell(controller: AppController, book: Book, state: BookCardState, showOwned: Boolean = false) {
    var menu by remember { mutableStateOf(false) }
    Column(Modifier.fillMaxWidth().padding(bottom = 18.dp)) {
        Box {
            Cover(
                book, controller.covers,
                Modifier.fillMaxWidth().bookClicks(book, onTap = { controller.showDetails(book) }, onLongPress = { menu = true }),
            )
            // Already on the reader's shelf: a small check, so Browse shows what they have at a glance.
            if (showOwned && state.onShelf) OwnedBadge(Modifier.align(Alignment.TopEnd))
            // Downloading: the cover dims under a progress ring, then clears when it's ready.
            DownloadScrim(state.downloading, state.fraction, Modifier.matchParentSize())
            if (isReading(state.progress)) {
                Box(Modifier.align(Alignment.BottomStart).fillMaxWidth().height(3.dp).background(Color.Black.copy(alpha = 0.25f))) {
                    Box(Modifier.fillMaxWidth((state.progress ?: 0.0).toFloat()).fillMaxHeight().background(MaterialTheme.colorScheme.secondary))
                }
            }
            if (state.downloaded && !state.downloading) ResumeButton(book, isReading(state.progress), Modifier.align(Alignment.BottomEnd).padding(6.dp)) { controller.open(book) }
            // No "⋮" in the narrow grid: press and hold opens the same menu, anchored here.
            BookMenu(controller, book, menu, { menu = it }, onDetails = { controller.showDetails(book) }, showButton = false)
        }
        Text(book.title, Modifier.padding(top = 6.dp), style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onBackground)
        Text(
            cardStatus(state, book), style = MaterialTheme.typography.labelMedium, maxLines = 1,
            color = when { state.error != null -> MaterialTheme.colorScheme.error; isReading(state.progress) -> MaterialTheme.colorScheme.secondary; else -> MaterialTheme.colorScheme.onSurfaceVariant },
        )
    }
}

/**
 * The play button on a downloaded book's cover: opens the book at the reader's place in one tap.
 * On a dark disc so it reads on any cover art; press and hold names it.
 */
@OptIn(androidx.compose.material3.ExperimentalMaterial3Api::class)
@Composable
internal fun ResumeButton(book: Book, reading: Boolean, modifier: Modifier = Modifier, small: Boolean = false, onClick: () -> Unit) {
    val label = if (reading) "Resume ${book.title}" else "Start ${book.title}"
    val press = remember { MutableInteractionSource() }
    androidx.compose.material3.TooltipBox(
        positionProvider = androidx.compose.material3.TooltipDefaults.rememberTooltipPositionProvider(androidx.compose.material3.TooltipAnchorPosition.Above),
        tooltip = { PlainTooltip { Text(if (reading) "Resume" else "Start reading") } },
        state = androidx.compose.material3.rememberTooltipState(),
        modifier = modifier,
    ) {
        Box(
            Modifier.size(if (small) 22.dp else 32.dp).pressScale(press, 0.88f).clip(CircleShape).background(Color.Black.copy(alpha = 0.55f))
                .clickable(press, ripple(), role = Role.Button, onClickLabel = label, onClick = onClick),
            contentAlignment = Alignment.Center,
        ) { Icon(BrandIcons.Play, label, Modifier.size(if (small) 11.dp else 15.dp).offset(x = 1.dp), tint = Color.White) }
    }
}

@Composable
internal fun OwnedBadge(modifier: Modifier = Modifier) {
    Box(modifier.padding(6.dp).size(22.dp).clip(CircleShape).background(MaterialTheme.colorScheme.secondary), contentAlignment = Alignment.Center) {
        Icon(BrandIcons.Check, "On your shelf", Modifier.size(14.dp), tint = MaterialTheme.colorScheme.onSecondary)
    }
}

@Composable
internal fun BookMenu(
    controller: AppController,
    book: Book,
    open: Boolean,
    onOpenChange: (Boolean) -> Unit,
    tint: Color = MaterialTheme.colorScheme.onSurfaceVariant,
    onDetails: () -> Unit,
    compact: Boolean = false,
    /** False where the card itself is the anchor (press and hold opens the menu, no "⋮"). */
    showButton: Boolean = true,
) {
    var confirmRemove by remember { mutableStateOf<Edition?>(null) }
    var addToListOpen by remember { mutableStateOf(false) }
    Box {
        if (showButton) IconButton(onClick = { onOpenChange(true) }, modifier = if (compact) Modifier.size(36.dp) else Modifier) { Icon(BrandIcons.MoreVertical, "More options for ${book.title}", Modifier.size(if (compact) 18.dp else 24.dp), tint = tint) }
        DropdownMenu(expanded = open, onDismissRequest = { onOpenChange(false) }) {
            // Read here, not in the card, so the catalog is only consulted while the menu is open.
            val catalog = controller.ui as? LibraryUiState.Catalog ?: return@DropdownMenu
            DropdownMenuItem(
                text = { Text("Details") },
                leadingIcon = { Icon(BrandIcons.Library, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                onClick = { onOpenChange(false); onDetails() },
            )
            DropdownMenuItem(
                text = { Text(if (isFinished(catalog.progress[book.id])) "Mark as unread" else "Mark as read") },
                leadingIcon = { Icon(BrandIcons.Check, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                onClick = { onOpenChange(false); controller.markRead(book, !isFinished(catalog.progress[book.id])) },
            )
            DropdownMenuItem(
                text = { Text("Add to list") },
                leadingIcon = { Icon(BrandIcons.Request, null, Modifier.size(18.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant) },
                onClick = { onOpenChange(false); addToListOpen = true },
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
                    onClick = { onOpenChange(false); if (available) confirmRemove = edition else controller.download(edition) },
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

/**
 * Tap shows the book's details; press and hold (or TalkBack's long-press action) opens its quick
 * actions with a haptic tick -- the same menu as the "⋮". Covers ([scale]) sink under the finger.
 */
@OptIn(ExperimentalFoundationApi::class)
internal fun Modifier.bookClicks(book: Book, scale: Boolean = true, onTap: () -> Unit, onLongPress: () -> Unit): Modifier = composed {
    val press = remember { MutableInteractionSource() }
    val haptics = LocalHapticFeedback.current
    (if (scale) pressScale(press) else this).combinedClickable(
        press, ripple(), onClickLabel = "Details for ${book.title}", role = Role.Button, onLongClickLabel = "Quick actions for ${book.title}",
        onLongClick = { haptics.performHapticFeedback(HapticFeedbackType.LongPress); onLongPress() }, onClick = onTap,
    )
}

@Composable
internal fun DownloadScrim(visible: Boolean, fraction: Float?, modifier: Modifier) {
    AnimatedVisibility(visible, modifier, enter = fadeIn(tween(200)), exit = fadeOut(tween(300))) {
        Box(Modifier.fillMaxSize().clip(RoundedCornerShape(6.dp)).background(Color.Black.copy(alpha = 0.45f)), contentAlignment = Alignment.Center) {
            DownloadRing(fraction, Modifier.size(36.dp), color = Color.White, strokeWidth = 3.dp)
        }
    }
}

/** Determinate once the size is known, spinning until then; the fill glides between updates. */
@Composable
internal fun DownloadRing(fraction: Float?, modifier: Modifier, color: Color = MaterialTheme.colorScheme.secondary, strokeWidth: androidx.compose.ui.unit.Dp = 2.dp) {
    if (fraction == null) { CircularProgressIndicator(modifier, strokeWidth = strokeWidth, color = color); return }
    val shown by animateFloatAsState(fraction, tween(250), label = "download")
    CircularProgressIndicator(progress = { shown }, modifier = modifier, strokeWidth = strokeWidth, color = color, trackColor = color.copy(alpha = 0.25f))
}

@Composable
internal fun Cover(book: Book, loader: CoverLoader, modifier: Modifier = Modifier) {
    // Straight from memory when it's there, so a cover scrolled back into view never flashes its
    // placeholder; only a cover that has to come from disk or the network fades in.
    val cached = remember(book.coverUrl, book.updatedAt) { loader.peek(book) }
    val bitmap by produceState(cached, book.coverUrl, book.updatedAt) { if (value == null) value = withContext(Dispatchers.IO) { loader.load(book) } }
    val alpha by animateFloatAsState(if (bitmap != null) 1f else 0f, tween(if (cached != null) 0 else 280), label = "cover")
    Box(
        modifier.aspectRatio(2f / 3f).clip(RoundedCornerShape(6.dp)).background(CoverGradient),
        contentAlignment = Alignment.Center,
    ) {
        if (alpha < 1f) Text(initialsOf(book.title), fontFamily = LiterataFamily, fontWeight = FontWeight.Bold, fontSize = 17.sp, color = Color.White)
        bitmap?.let { Image(it, contentDescription = null, Modifier.fillMaxSize().graphicsLayer { this.alpha = alpha }, contentScale = ContentScale.Crop) }
    }
}

internal val CoverGradient = Brush.linearGradient(listOf(HarborNavy, Color(0xFF315B72)))

/** A cover with its title and author, for Browse's horizontal rows. */
@Composable
internal fun BookShelfCard(controller: AppController, book: Book, state: BookCardState, modifier: Modifier = Modifier) {
    var menu by remember { mutableStateOf(false) }
    Column(modifier.width(112.dp)) {
        Box {
            Cover(book, controller.covers, Modifier.fillMaxWidth().bookClicks(book, onTap = { controller.showDetails(book) }, onLongPress = { menu = true }))
            if (state.onShelf) OwnedBadge(Modifier.align(Alignment.TopEnd))
            if (state.downloaded) ResumeButton(book, isReading(state.progress), Modifier.align(Alignment.BottomEnd).padding(6.dp)) { controller.open(book) }
            BookMenu(controller, book, menu, { menu = it }, onDetails = { controller.showDetails(book) }, showButton = false)
        }
        Text(book.title, Modifier.padding(top = 8.dp), style = MaterialTheme.typography.labelLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
        Text(
            if (book.series.isNotBlank() && book.seriesIndex > 0) "Book ${seriesLabel(book).substringAfterLast('#')}" else book.authors.firstOrNull().orEmpty(),
            style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis,
        )
    }
}
