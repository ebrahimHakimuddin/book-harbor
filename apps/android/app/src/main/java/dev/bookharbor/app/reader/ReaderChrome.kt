package dev.bookharbor.app.reader

import android.app.Activity
import android.view.accessibility.AccessibilityManager
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.size
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.text.font.FontStyle
import androidx.lifecycle.compose.LifecycleResumeEffect
import android.text.format.DateFormat
import dev.bookharbor.app.sync.Locator
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import java.time.LocalTime
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Slider
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.State
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import dev.bookharbor.app.ui.BrandIcons
import dev.bookharbor.app.ui.theme.BookHarborTheme
import dev.bookharbor.app.ui.theme.LiterataFamily
import kotlin.math.roundToInt

/**
 * The reading chrome shared by every format: themed surface, title bar, progress footer,
 * settings and contents sheets, and the brightness override. [content] is the book itself.
 *
 * [fixedLayout] hides typography controls for formats (PDF) that cannot reflow.
 */
@Composable
fun ReaderScaffold(
    state: ReaderState,
    onAction: (ReaderAction) -> Unit,
    onClose: () -> Unit,
    progressLabel: String,
    /** The footer bar's fill and percentage. Defaults to the whole book; pass the current
     * chapter's own fraction where "chapter" is a meaningful sub-unit (e.g. EPUB). */
    progress: Float = state.overallProgress,
    percentage: Int = state.overallPercentage,
    fixedLayout: Boolean = false,
    contentsLabel: String = "Contents",
    /** True while the content is actively scrolling, so the chrome can duck out of the way. */
    isScrolling: Boolean = false,
    annotations: BookAnnotations,
    /** The bookmarkable current position and its list label, once known. */
    here: Pair<Locator, String>?,
    onGoTo: (Locator) -> Unit,
    /** Full-text search over the book, or null where the format/platform can't extract text. */
    search: (suspend (String) -> List<SearchHit>)?,
    stats: ReadingStats,
    content: @Composable (PaddingValues) -> Unit,
) {
    var chromeVisible by remember { mutableStateOf(true) }
    LifecycleResumeEffect(stats) {
        stats.start()
        onPauseOrDispose { stats.stop() }
    }
    val hour by produceState(LocalTime.now().hour) { while (true) { delay(60_000); value = LocalTime.now().hour } }
    val effective = state.settings.effectiveAt(hour)
    // The sleep timer holds the screen awake until it fires, then closes the book (saving the
    // place) so the device can sleep normally.
    var sleepMinutes by remember { mutableIntStateOf(0) }
    var sleepAt by remember { mutableStateOf<Long?>(null) }
    val view = LocalView.current
    DisposableEffect(sleepAt) {
        view.keepScreenOn = sleepAt != null
        onDispose { view.keepScreenOn = false }
    }
    LaunchedEffect(sleepAt) {
        val at = sleepAt ?: return@LaunchedEffect
        delay(at - System.currentTimeMillis())
        onClose()
    }
    val bookmark = here?.let { annotations.find(AnnotationKind.Bookmark, it.first) }
    // TalkBack's touch exploration turns single taps into "focus this element" rather than a
    // plain gesture, so the tap-to-show zone below often can't be reached once the chrome is
    // hidden -- a TalkBack user could lose the close/contents/settings buttons with no reliable
    // way to bring them back (aside from the system Back gesture). Never auto-hide for them.
    val touchExplorationEnabled by rememberTouchExplorationEnabled()
    LaunchedEffect(isScrolling, touchExplorationEnabled) {
        if (touchExplorationEnabled) chromeVisible = true else if (isScrolling) chromeVisible = false
    }
    ReaderFullscreen(chromeVisible)
    BookHarborTheme(readerTheme = effective.theme) {
        BrightnessEffect(effective.brightness)
        Scaffold(
            containerColor = MaterialTheme.colorScheme.background,
            contentWindowInsets = WindowInsets(0),
            topBar = {
                AnimatedVisibility(
                    visible = chromeVisible,
                    enter = fadeIn(tween(220)) + slideInVertically(tween(220)) { -it },
                    exit = fadeOut(tween(220)) + slideOutVertically(tween(220)) { -it },
                ) {
                    ReaderTopBar(
                        state.bookTitle, onClose, onContents = { onAction(ReaderAction.OpenContents) }, contentsLabel = contentsLabel, onSettings = { onAction(ReaderAction.OpenSettings) },
                        bookmarked = bookmark != null,
                        onBookmark = here?.let { (locator, label) -> { annotations.toggleBookmark(locator, label) } },
                    )
                }
            },
            bottomBar = {
                AnimatedVisibility(
                    visible = chromeVisible && state.settings.showProgress,
                    enter = fadeIn(tween(220)) + slideInVertically(tween(220)) { it },
                    exit = fadeOut(tween(220)) + slideOutVertically(tween(220)) { it },
                ) {
                    val unit = if (fixedLayout) "page" else "chapter"
                    ReaderProgressBar(
                        progress = progress, label = progressLabel, percentage = percentage,
                        onPrevious = { onAction(ReaderAction.SelectChapter(state.chapterIndex - 1)) }.takeIf { state.chapterIndex > 0 },
                        onNext = { onAction(ReaderAction.SelectChapter(state.chapterIndex + 1)) }.takeIf { state.chapterIndex < state.chapters.lastIndex },
                        previousLabel = "Previous $unit", nextLabel = "Next $unit",
                    )
                }
            },
            content = { padding ->
                // A tap in the middle band shows/hides the chrome; a link or other clickable
                // span inside the content consumes its own tap first, so this never fires for it.
                Box(
                    Modifier.fillMaxSize().pointerInput(touchExplorationEnabled) {
                        detectTapGestures(onTap = { offset ->
                            if (!touchExplorationEnabled && offset.x in size.width * 0.3f..size.width * 0.7f) chromeVisible = !chromeVisible
                        })
                    },
                ) { content(padding) }
            },
        )
        if (state.settingsOpen) {
            ReaderSettingsSheet(
                state.settings, fixedLayout, onAction, stats,
                sleepMinutes = sleepMinutes, sleepAt = sleepAt,
                onSleep = { minutes -> sleepMinutes = minutes; sleepAt = if (minutes == 0) null else System.currentTimeMillis() + minutes * 60_000L },
            )
        }
        if (state.contentsOpen) {
            ContentsSheet(state, contentsLabel, onAction, annotations, search, onGoTo = { onGoTo(it); onAction(ReaderAction.CloseContents) })
        }
    }
}

/** Hides the system status/navigation bars in step with the reader chrome, so hiding chrome
 * gives a genuinely fullscreen page rather than just a page with no title bar. A swipe from the
 * screen edge still reveals the system bars transiently, per platform convention. */
@Composable
private fun ReaderFullscreen(chromeVisible: Boolean) {
    val view = LocalView.current
    if (view.isInEditMode) return
    DisposableEffect(view) {
        val controller = WindowCompat.getInsetsController((view.context as Activity).window, view)
        controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        onDispose { controller.show(WindowInsetsCompat.Type.systemBars()) }
    }
    LaunchedEffect(chromeVisible) {
        val controller = WindowCompat.getInsetsController((view.context as Activity).window, view)
        if (chromeVisible) controller.show(WindowInsetsCompat.Type.systemBars()) else controller.hide(WindowInsetsCompat.Type.systemBars())
    }
}

/** Tracks whether a touch-exploration accessibility service (e.g. TalkBack) is active, live. */
@Composable
private fun rememberTouchExplorationEnabled(): State<Boolean> {
    val context = LocalContext.current
    val manager = remember(context) { context.getSystemService(AccessibilityManager::class.java) }
    val state = remember(manager) { mutableStateOf(manager?.isTouchExplorationEnabled == true) }
    DisposableEffect(manager) {
        if (manager == null) return@DisposableEffect onDispose {}
        val listener = AccessibilityManager.TouchExplorationStateChangeListener { enabled -> state.value = enabled }
        manager.addTouchExplorationStateChangeListener(listener)
        onDispose { manager.removeTouchExplorationStateChangeListener(listener) }
    }
    return state
}

@Composable
private fun BrightnessEffect(brightness: Float?) {
    val window = (LocalContext.current as? Activity)?.window ?: return
    DisposableEffect(brightness) {
        val original = window.attributes.screenBrightness
        window.attributes = window.attributes.apply { screenBrightness = brightness ?: android.view.WindowManager.LayoutParams.BRIGHTNESS_OVERRIDE_NONE }
        onDispose { window.attributes = window.attributes.apply { screenBrightness = original } }
    }
}

@Composable
private fun ReaderTopBar(bookTitle: String, onClose: () -> Unit, onContents: () -> Unit, contentsLabel: String, onSettings: () -> Unit, bookmarked: Boolean, onBookmark: (() -> Unit)?) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.background)
            .padding(WindowInsets.statusBars.asPaddingValues())
            .height(58.dp)
            .padding(horizontal = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IconButton(onClick = onClose) {
            Icon(imageVector = Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Close reader")
        }
        Text(bookTitle, Modifier.weight(1f), style = MaterialTheme.typography.titleMedium, textAlign = TextAlign.Center, maxLines = 1, overflow = TextOverflow.Ellipsis)
        IconButton(onClick = { onBookmark?.invoke() }, enabled = onBookmark != null) {
            Icon(
                if (bookmarked) BrandIcons.BookmarkAdded else BrandIcons.Bookmark,
                if (bookmarked) "Remove bookmark" else "Bookmark this place",
                tint = if (bookmarked) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.onSurface,
            )
        }
        TextButton(onClick = onContents, modifier = Modifier.semantics { contentDescription = contentsLabel }) {
            Text(contentsLabel, style = MaterialTheme.typography.labelLarge)
        }
        TextButton(onClick = onSettings, modifier = Modifier.semantics { contentDescription = "Reading settings" }) {
            Text("Aa", fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 17.sp)
        }
    }
}

@Composable
private fun ReaderProgressBar(
    progress: Float,
    label: String,
    percentage: Int,
    onPrevious: (() -> Unit)?,
    onNext: (() -> Unit)?,
    previousLabel: String,
    nextLabel: String,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.background)
            .padding(bottom = WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()),
    ) {
        Box(Modifier.fillMaxWidth().height(2.dp).background(MaterialTheme.colorScheme.surfaceVariant)) {
            Box(Modifier.fillMaxWidth(progress.coerceIn(0f, 1f)).height(2.dp).background(MaterialTheme.colorScheme.secondary))
        }
        Row(
            modifier = Modifier.fillMaxWidth().height(48.dp).padding(horizontal = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = { onPrevious?.invoke() }, enabled = onPrevious != null) {
                Icon(BrandIcons.ChevronLeft, previousLabel, tint = if (onPrevious != null) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.35f))
            }
            Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, modifier = Modifier.weight(1f))
            Text("$percentage%", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(end = 6.dp))
            IconButton(onClick = { onNext?.invoke() }, enabled = onNext != null) {
                Icon(BrandIcons.ChevronRight, nextLabel, tint = if (onNext != null) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.35f))
            }
        }
    }
}

/** The deliberate end-of-chapter card: reaching the bottom never advances by itself. */
@Composable
fun ChapterTransition(nextChapter: ReaderChapter?, onNextChapter: () -> Unit, onFinishBook: () -> Unit, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .padding(horizontal = 24.dp, vertical = 30.dp),
    ) {
        Text("CHAPTER COMPLETE", color = MaterialTheme.colorScheme.secondary, style = MaterialTheme.typography.labelMedium, letterSpacing = 1.7.sp)
        Spacer(Modifier.height(12.dp))
        Text(nextChapter?.title ?: "You reached the final page", style = MaterialTheme.typography.headlineMedium)
        Spacer(Modifier.height(22.dp))
        // The last chapter's action actually finishes the book (records 100% and returns to the
        // library) rather than ending on a dead, disabled button.
        Button(
            onClick = if (nextChapter == null) onFinishBook else onNextChapter,
            shape = RoundedCornerShape(12.dp),
            colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary),
        ) {
            Text(if (nextChapter == null) "Finish book" else "Next chapter")
        }
    }
}

private enum class SheetTab { Contents, Notes, Search }

/** Contents, the book's bookmarks/highlights, and in-book search, as tabs of one sheet. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ContentsSheet(
    state: ReaderState,
    label: String,
    onAction: (ReaderAction) -> Unit,
    annotations: BookAnnotations,
    search: (suspend (String) -> List<SearchHit>)?,
    onGoTo: (Locator) -> Unit,
) {
    var tab by rememberSaveable { mutableStateOf(SheetTab.Contents) }
    ModalBottomSheet(
        onDismissRequest = { onAction(ReaderAction.CloseContents) },
        containerColor = MaterialTheme.colorScheme.surface,
        contentWindowInsets = { WindowInsets.ime },
    ) {
        Box(Modifier.padding(horizontal = 24.dp)) {
            ChoiceRow(SheetTab.entries, tab, { if (it == SheetTab.Contents) label else it.name }) { tab = it }
        }
        Box(Modifier.heightIn(min = 240.dp, max = 520.dp).padding(bottom = 24.dp)) {
            when (tab) {
                SheetTab.Contents -> LazyColumn {
                    itemsIndexed(state.chapters) { index, chapter ->
                        val current = index == state.chapterIndex
                        Row(
                            Modifier.fillMaxWidth().clickable { onAction(ReaderAction.SelectChapter(index)) }.padding(horizontal = 24.dp, vertical = 14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text("${index + 1}", Modifier.padding(end = 16.dp), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.secondary)
                            Text(chapter.title, Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge, fontWeight = if (current) FontWeight.Bold else FontWeight.Normal)
                            if (current) Text("Reading", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
                        }
                    }
                }
                SheetTab.Notes -> NotesList(annotations, onGoTo)
                SheetTab.Search -> SearchPanel(search, onGoTo)
            }
        }
    }
}

@Composable
private fun NotesList(annotations: BookAnnotations, onGoTo: (Locator) -> Unit) {
    val context = LocalContext.current
    val items = remember(annotations.items) { annotations.items.sortedWith(InReadingOrder) }
    var editing by remember { mutableStateOf<Annotation?>(null) }
    if (items.isEmpty()) {
        Text(
            "No bookmarks or highlights yet. Tap the bookmark icon to mark your place, or press and hold a paragraph to highlight it.",
            Modifier.padding(24.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        return
    }
    LazyColumn {
        item {
            TextButton(onClick = { shareText(context, exportAnnotations(annotations.bookTitle, items)) }, Modifier.padding(horizontal = 12.dp)) {
                Icon(BrandIcons.Share, null, Modifier.size(18.dp))
                Text("  Export all")
            }
        }
        items(items, key = { it.id }) { item ->
            var menu by remember { mutableStateOf(false) }
            Row(Modifier.fillMaxWidth().clickable { onGoTo(item.locator) }.padding(start = 24.dp, end = 8.dp, top = 12.dp, bottom = 12.dp)) {
                Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text(
                        (if (item.kind == AnnotationKind.Bookmark) "Bookmark · " else "Highlight · ") + item.label,
                        style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary,
                    )
                    if (item.excerpt.isNotBlank()) Text(item.excerpt, style = MaterialTheme.typography.bodyMedium, fontStyle = FontStyle.Italic, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    if (item.note.isNotBlank()) Text(item.note, style = MaterialTheme.typography.bodyMedium)
                }
                Box {
                    IconButton(onClick = { menu = true }) { Icon(BrandIcons.MoreVertical, "Options for this ${item.kind.name.lowercase()}") }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(text = { Text(if (item.note.isBlank()) "Add note" else "Edit note") }, onClick = { menu = false; editing = item })
                        if (item.kind == AnnotationKind.Highlight) {
                            DropdownMenuItem(text = { Text("Share") }, onClick = { menu = false; shareText(context, quote(item.excerpt, annotations.bookTitle, item.label)) })
                        }
                        DropdownMenuItem(text = { Text("Delete", color = MaterialTheme.colorScheme.error) }, onClick = { menu = false; annotations.delete(item) })
                    }
                }
            }
        }
    }
    editing?.let { target -> NoteDialog(target.note, onSave = { annotations.setNote(target, it); editing = null }, onDismiss = { editing = null }) }
}

/** A highlight formatted for sharing outside the app. */
internal fun quote(excerpt: String, bookTitle: String, label: String) = "“${excerpt.trim()}”\n— $bookTitle, $label"

@Composable
internal fun NoteDialog(initial: String, onSave: (String) -> Unit, onDismiss: () -> Unit) {
    var text by remember { mutableStateOf(initial) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Note") },
        text = { OutlinedTextField(text, { text = it }, Modifier.fillMaxWidth(), minLines = 3, placeholder = { Text("Your thoughts on this passage") }) },
        confirmButton = { TextButton(onClick = { onSave(text) }) { Text("Save") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
    )
}

@OptIn(FlowPreview::class)
@Composable
private fun SearchPanel(search: (suspend (String) -> List<SearchHit>)?, onGoTo: (Locator) -> Unit) {
    if (search == null) {
        Text("Search needs Android 15 or later for PDFs.", Modifier.padding(24.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        return
    }
    var query by rememberSaveable { mutableStateOf("") }
    var hits by remember { mutableStateOf<List<SearchHit>?>(emptyList()) }
    LaunchedEffect(Unit) {
        snapshotFlow { query.trim() }.debounce(350).distinctUntilChanged().collectLatest { q ->
            if (q.length < 2) { hits = emptyList(); return@collectLatest }
            hits = null
            hits = runCatching { search(q) }.getOrDefault(emptyList())
        }
    }
    Column {
        OutlinedTextField(
            query, { query = it }, Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 8.dp), singleLine = true,
            placeholder = { Text("Search in this book") }, leadingIcon = { Icon(BrandIcons.Search, null, Modifier.size(20.dp)) },
            shape = RoundedCornerShape(12.dp),
        )
        val found = hits
        when {
            found == null -> Box(Modifier.fillMaxWidth().padding(24.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
            found.isEmpty() && query.trim().length >= 2 -> Text("No matches.", Modifier.padding(24.dp), color = MaterialTheme.colorScheme.onSurfaceVariant)
            else -> LazyColumn {
                if (found.size >= MAX_SEARCH_HITS) item { Text("Showing the first $MAX_SEARCH_HITS matches.", Modifier.padding(horizontal = 24.dp), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                items(found) { hit ->
                    Column(Modifier.fillMaxWidth().clickable { onGoTo(hit.locator) }.padding(horizontal = 24.dp, vertical = 12.dp)) {
                        Text(hit.label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.secondary)
                        Text(hit.snippet, style = MaterialTheme.typography.bodyMedium, maxLines = 3, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
        }
    }
}

const val MAX_SEARCH_HITS = 200

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ReaderSettingsSheet(
    settings: ReaderSettings,
    fixedLayout: Boolean,
    onAction: (ReaderAction) -> Unit,
    stats: ReadingStats,
    sleepMinutes: Int,
    sleepAt: Long?,
    onSleep: (Int) -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = { onAction(ReaderAction.CloseSettings) },
        containerColor = MaterialTheme.colorScheme.surface,
        contentWindowInsets = { WindowInsets.ime },
    ) {
        val context = LocalContext.current
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).padding(bottom = 32.dp)) {
            Text("Reading settings", style = MaterialTheme.typography.headlineMedium)
            val minutes = remember { stats.secondsToday() / 60 }
            val streak = remember { stats.streakDays() }
            Text(
                "Today: $minutes min" + if (streak > 1) " · $streak-day streak" else "",
                Modifier.padding(top = 4.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.secondary,
            )
            Spacer(Modifier.height(24.dp))

            SettingLabel("Theme")
            ChoiceRow(ReaderTheme.entries, settings.theme, ReaderTheme::label) { onAction(ReaderAction.SelectTheme(it)) }

            if (!fixedLayout) {
                Spacer(Modifier.height(24.dp))
                SettingLabel("Typeface")
                ChoiceRow(ReaderTypeface.entries, settings.typeface, ReaderTypeface::label) { onAction(ReaderAction.SelectTypeface(it)) }
                Spacer(Modifier.height(24.dp))
                SettingLabel("Alignment")
                ChoiceRow(ReaderAlignment.entries, settings.alignment, ReaderAlignment::label) { onAction(ReaderAction.SelectAlignment(it)) }
                Spacer(Modifier.height(24.dp))
                SettingSlider("Text size", "${(settings.fontScale * 100).roundToInt()}%", settings.fontScale, 0.8f..1.5f) { onAction(ReaderAction.SetFontScale(it)) }
                SettingSlider("Line height", String.format("%.2f", settings.lineHeight), settings.lineHeight, 1.25f..2f) { onAction(ReaderAction.SetLineHeight(it)) }
                SettingSlider("Paragraph spacing", String.format("%.1f×", settings.paragraphSpacing), settings.paragraphSpacing, 0.5f..2f) { onAction(ReaderAction.SetParagraphSpacing(it)) }
                SettingSlider("Margins", "${settings.horizontalMargin} dp", settings.horizontalMargin.toFloat(), 16f..48f) { onAction(ReaderAction.SetMargin(it.roundToInt())) }
            }

            Row(Modifier.fillMaxWidth().padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    SettingLabel("Screen brightness")
                    Text(if (settings.brightness == null) "Following the system" else "${((settings.brightness) * 100).roundToInt()}%", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = settings.brightness != null, onCheckedChange = { onAction(ReaderAction.SetBrightness(if (it) 0.6f else null)) })
            }
            settings.brightness?.let { value -> Slider(value = value, onValueChange = { onAction(ReaderAction.SetBrightness(it)) }, valueRange = 0.05f..1f) }

            Row(Modifier.fillMaxWidth().padding(top = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    SettingLabel("Show reading progress")
                    Text("Chapter or page, and overall position", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = settings.showProgress, onCheckedChange = { onAction(ReaderAction.SetProgressVisible(it)) })
            }

            Row(Modifier.fillMaxWidth().padding(top = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    SettingLabel("Night schedule")
                    Text("Dark theme and a dimmer screen at night", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = settings.nightSchedule, onCheckedChange = { onAction(ReaderAction.SetNightSchedule(it)) })
            }
            if (settings.nightSchedule) {
                val hourLabel = { hour: Int -> DateFormat.getTimeFormat(context).format(java.util.Calendar.getInstance().apply { set(java.util.Calendar.HOUR_OF_DAY, hour); set(java.util.Calendar.MINUTE, 0) }.time) }
                SettingSlider("Starts", hourLabel(settings.nightStart), settings.nightStart.toFloat(), 0f..23f, steps = 22) { onAction(ReaderAction.SetNightHours(it.roundToInt(), settings.nightEnd)) }
                SettingSlider("Ends", hourLabel(settings.nightEnd), settings.nightEnd.toFloat(), 0f..23f, steps = 22) { onAction(ReaderAction.SetNightHours(settings.nightStart, it.roundToInt())) }
                SettingSlider("Night brightness", "${(settings.nightBrightness * 100).roundToInt()}%", settings.nightBrightness, 0.05f..1f) { onAction(ReaderAction.SetNightBrightness(it)) }
            }

            Spacer(Modifier.height(16.dp))
            SettingLabel("Sleep timer")
            ChoiceRow(listOf(0, 15, 30, 60), sleepMinutes, { if (it == 0) "Off" else "$it min" }, onSleep)
            if (sleepAt != null) {
                val time = remember(sleepAt) { DateFormat.getTimeFormat(context).format(java.util.Date(sleepAt)) }
                Text("Keeps the screen on, then closes the book at $time", Modifier.padding(top = 4.dp), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }

            TextButton(onClick = { onAction(ReaderAction.ResetSettings) }, Modifier.padding(top = 12.dp)) { Text("Reset to defaults") }
        }
    }
}

@Composable
private fun SettingLabel(label: String) = Text(label, style = MaterialTheme.typography.titleMedium)

@Composable
private fun <T> ChoiceRow(choices: List<T>, selected: T, label: (T) -> String, onSelected: (T) -> Unit) {
    Row(Modifier.fillMaxWidth().padding(top = 10.dp), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        choices.forEach { choice ->
            TextButton(
                onClick = { onSelected(choice) },
                modifier = Modifier.weight(1f).heightIn(min = 48.dp),
                colors = ButtonDefaults.textButtonColors(
                    containerColor = if (choice == selected) MaterialTheme.colorScheme.primary else Color.Transparent,
                    contentColor = if (choice == selected) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                ),
                // Pill-shaped, matching the other selector chips in the app (e.g. the library's
                // shelf filters), rather than a one-off corner radius used nowhere else.
                shape = CircleShape,
            ) { Text(label(choice), maxLines = 1, fontSize = 12.sp) }
        }
    }
}

@Composable
private fun SettingSlider(label: String, valueLabel: String, value: Float, range: ClosedFloatingPointRange<Float>, steps: Int = 0, onValueChange: (Float) -> Unit) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        SettingLabel(label)
        Text(valueLabel, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    Slider(value = value, onValueChange = onValueChange, valueRange = range, steps = steps)
}
