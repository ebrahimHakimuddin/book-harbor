package dev.bookharbor.app.reader

import android.app.Activity
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.lazy.itemsIndexed
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
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
    content: @Composable (PaddingValues) -> Unit,
) {
    BookHarborTheme(readerTheme = state.settings.theme) {
        BrightnessEffect(state.settings.brightness)
        Scaffold(
            containerColor = MaterialTheme.colorScheme.background,
            contentWindowInsets = WindowInsets(0),
            topBar = {
                ReaderTopBar(state.bookTitle, onClose, onContents = { onAction(ReaderAction.OpenContents) }, contentsLabel = contentsLabel, onSettings = { onAction(ReaderAction.OpenSettings) })
            },
            bottomBar = {
                AnimatedVisibility(visible = state.settings.showProgress, enter = fadeIn(), exit = fadeOut()) {
                    ReaderProgressBar(progress = progress, label = progressLabel, percentage = percentage)
                }
            },
            content = content,
        )
        if (state.settingsOpen) ReaderSettingsSheet(state.settings, fixedLayout, onAction)
        if (state.contentsOpen) ContentsSheet(state, contentsLabel, onAction)
    }
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
private fun ReaderTopBar(bookTitle: String, onClose: () -> Unit, onContents: () -> Unit, contentsLabel: String, onSettings: () -> Unit) {
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
        TextButton(onClick = onContents, modifier = Modifier.semantics { contentDescription = contentsLabel }) {
            Text(contentsLabel, style = MaterialTheme.typography.labelLarge)
        }
        TextButton(onClick = onSettings, modifier = Modifier.semantics { contentDescription = "Reading settings" }) {
            Text("Aa", fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 17.sp)
        }
    }
}

@Composable
private fun ReaderProgressBar(progress: Float, label: String, percentage: Int) {
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
            modifier = Modifier.fillMaxWidth().height(42.dp).padding(horizontal = 18.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, modifier = Modifier.weight(1f))
            Text("$percentage%", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** The deliberate end-of-chapter card: reaching the bottom never advances by itself. */
@Composable
fun ChapterTransition(nextChapter: ReaderChapter?, onNextChapter: () -> Unit, modifier: Modifier = Modifier) {
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
        Button(
            onClick = onNextChapter,
            enabled = nextChapter != null,
            shape = RoundedCornerShape(12.dp),
            colors = ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.primary),
        ) {
            Text(if (nextChapter == null) "Book finished" else "Next chapter")
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ContentsSheet(state: ReaderState, label: String, onAction: (ReaderAction) -> Unit) {
    ModalBottomSheet(onDismissRequest = { onAction(ReaderAction.CloseContents) }, containerColor = MaterialTheme.colorScheme.surface) {
        Text(label, Modifier.padding(horizontal = 24.dp, vertical = 8.dp), style = MaterialTheme.typography.headlineMedium)
        LazyColumn(Modifier.heightIn(max = 520.dp).padding(bottom = 24.dp)) {
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
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ReaderSettingsSheet(settings: ReaderSettings, fixedLayout: Boolean, onAction: (ReaderAction) -> Unit) {
    ModalBottomSheet(
        onDismissRequest = { onAction(ReaderAction.CloseSettings) },
        containerColor = MaterialTheme.colorScheme.surface,
        contentWindowInsets = { WindowInsets.ime },
    ) {
        Column(Modifier.fillMaxWidth().padding(horizontal = 24.dp).padding(bottom = 32.dp)) {
            Text("Reading settings", style = MaterialTheme.typography.headlineMedium)
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
                shape = RoundedCornerShape(10.dp),
            ) { Text(label(choice), maxLines = 1, fontSize = 12.sp) }
        }
    }
}

@Composable
private fun SettingSlider(label: String, valueLabel: String, value: Float, range: ClosedFloatingPointRange<Float>, onValueChange: (Float) -> Unit) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
        SettingLabel(label)
        Text(valueLabel, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    Slider(value = value, onValueChange = onValueChange, valueRange = range)
}
