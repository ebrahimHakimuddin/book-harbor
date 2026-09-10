/*
THESIS: The book owns the screen; BookHarbor's identity guides without enclosing prose in generic cards.
OWN-WORLD: Sand reading fields, Harbor Navy type, Sea Teal state, Mist Cyan rules, Sunrise used only for warmth; Literata carries reading and Inter carries controls.
STORY: A reader stays oriented, reads one continuously scrolling chapter, then deliberately enters the next and can tune the page without a network.
FIRST VIEWPORT: Quiet title chrome sits above a wide Literata text measure; chapter position begins the page and a two-pixel progress line anchors the bottom.
FORM: The user-supplied BookHarbor brand kit is visual authority across the Android reader.
FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
*/
package dev.bookharbor.app.reader

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.asPaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBars
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
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
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.bookharbor.app.ui.theme.InterFamily
import dev.bookharbor.app.ui.theme.LiterataFamily
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlin.math.roundToInt

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ReaderScreen(
    state: ReaderState,
    onClose: () -> Unit,
    onAction: (ReaderAction) -> Unit,
) {
    val scrollState = rememberScrollState()

    LaunchedEffect(state.chapter.id) {
        scrollState.scrollTo(0)
    }
    LaunchedEffect(scrollState, state.chapter.id) {
        snapshotFlow { scrollState.value to scrollState.maxValue }
            .map { (value, maximum) ->
                if (maximum == 0) 0f else value.toFloat() / maximum.toFloat()
            }
            .map { (it * 200).roundToInt() / 200f }
            .distinctUntilChanged()
            .collect { onAction(ReaderAction.RecordProgress(it)) }
    }

    Scaffold(
        containerColor = MaterialTheme.colorScheme.background,
        contentWindowInsets = WindowInsets(0),
        topBar = {
            ReaderTopBar(
                bookTitle = state.bookTitle,
                onClose = onClose,
                onSettings = { onAction(ReaderAction.OpenSettings) },
            )
        },
        bottomBar = {
            AnimatedVisibility(
                visible = state.settings.showProgress,
                enter = fadeIn(),
                exit = fadeOut(),
            ) {
                ReaderProgressBar(state = state)
            }
        },
    ) { scaffoldPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(scaffoldPadding)
                .verticalScroll(scrollState),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            ChapterContent(
                state = state,
                onNextChapter = { onAction(ReaderAction.NextChapter) },
            )
        }
    }

    if (state.settingsOpen) {
        ReaderSettingsSheet(
            settings = state.settings,
            onAction = onAction,
        )
    }
}

@Composable
private fun ReaderTopBar(
    bookTitle: String,
    onClose: () -> Unit,
    onSettings: () -> Unit,
) {
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
            Icon(
                imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                contentDescription = "Close reader",
            )
        }
        Text(
            text = bookTitle,
            modifier = Modifier.weight(1f),
            style = MaterialTheme.typography.titleMedium,
            textAlign = TextAlign.Center,
            maxLines = 1,
        )
        TextButton(
            onClick = onSettings,
            modifier = Modifier.semantics { contentDescription = "Reading settings" },
        ) {
            Text(
                text = "Aa",
                fontFamily = LiterataFamily,
                fontWeight = FontWeight.SemiBold,
                fontSize = 17.sp,
            )
        }
    }
}

@Composable
private fun ChapterContent(
    state: ReaderState,
    onNextChapter: () -> Unit,
) {
    val settings = state.settings
    val bodyFont: FontFamily = when (settings.typeface) {
        ReaderTypeface.Publisher, ReaderTypeface.Literata -> LiterataFamily
        ReaderTypeface.Inter -> InterFamily
    }
    val fontSize = 18.sp * settings.fontScale
    val bodyStyle = TextStyle(
        color = MaterialTheme.colorScheme.onBackground,
        fontFamily = bodyFont,
        fontSize = fontSize,
        lineHeight = fontSize * settings.lineHeight,
        fontWeight = FontWeight.Normal,
    )

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .widthIn(max = 680.dp)
            .padding(horizontal = settings.horizontalMargin.dp),
    ) {
        Spacer(Modifier.height(30.dp))
        Text(
            text = "CHAPTER ${state.chapterIndex + 1} OF ${state.chapters.size}",
            color = MaterialTheme.colorScheme.secondary,
            style = MaterialTheme.typography.labelMedium,
            letterSpacing = 1.7.sp,
        )
        HorizontalDivider(
            modifier = Modifier.padding(top = 12.dp, bottom = 26.dp),
            color = MaterialTheme.colorScheme.outline.copy(alpha = 0.62f),
        )
        Text(
            text = state.chapter.title,
            style = MaterialTheme.typography.headlineLarge,
        )
        Spacer(Modifier.height(34.dp))
        state.chapter.paragraphs.forEachIndexed { index, paragraph ->
            Text(
                text = paragraph,
                style = bodyStyle,
            )
            Spacer(Modifier.height(if (index == state.chapter.paragraphs.lastIndex) 72.dp else 24.dp))
        }
        ChapterTransition(
            nextChapter = state.nextChapter,
            onNextChapter = onNextChapter,
        )
        Spacer(Modifier.height(48.dp))
    }
}

@Composable
private fun ChapterTransition(
    nextChapter: ReaderChapter?,
    onNextChapter: () -> Unit,
) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .padding(horizontal = 24.dp, vertical = 30.dp),
    ) {
        Text(
            text = "CHAPTER COMPLETE",
            color = MaterialTheme.colorScheme.secondary,
            style = MaterialTheme.typography.labelMedium,
            letterSpacing = 1.7.sp,
        )
        Spacer(Modifier.height(12.dp))
        Text(
            text = nextChapter?.title ?: "You reached the final page",
            style = MaterialTheme.typography.headlineMedium,
        )
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

@Composable
private fun ReaderProgressBar(state: ReaderState) {
    Column(
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.background)
            .padding(bottom = WindowInsets.navigationBars.asPaddingValues().calculateBottomPadding()),
    ) {
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .height(2.dp)
                .background(MaterialTheme.colorScheme.surfaceVariant),
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth(state.overallProgress.coerceIn(0f, 1f))
                    .height(2.dp)
                    .background(MaterialTheme.colorScheme.secondary),
            )
        }
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .height(42.dp)
                .padding(horizontal = 18.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = "Chapter ${state.chapterIndex + 1} of ${state.chapters.size}",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                text = "${state.overallPercentage}%",
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ReaderSettingsSheet(
    settings: ReaderSettings,
    onAction: (ReaderAction) -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = { onAction(ReaderAction.CloseSettings) },
        containerColor = MaterialTheme.colorScheme.surface,
        contentWindowInsets = { WindowInsets.ime },
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp)
                .padding(bottom = 32.dp),
        ) {
            Text("Reading settings", style = MaterialTheme.typography.headlineMedium)
            Spacer(Modifier.height(24.dp))

            SettingLabel("Theme")
            ChoiceRow(
                choices = ReaderTheme.entries,
                selected = settings.theme,
                label = ReaderTheme::label,
                onSelected = { onAction(ReaderAction.SelectTheme(it)) },
            )

            Spacer(Modifier.height(24.dp))
            SettingLabel("Typeface")
            ChoiceRow(
                choices = ReaderTypeface.entries,
                selected = settings.typeface,
                label = ReaderTypeface::label,
                onSelected = { onAction(ReaderAction.SelectTypeface(it)) },
            )

            Spacer(Modifier.height(24.dp))
            SettingSlider(
                label = "Text size",
                valueLabel = "${(settings.fontScale * 100).roundToInt()}%",
                value = settings.fontScale,
                range = 0.8f..1.5f,
                onValueChange = { onAction(ReaderAction.SetFontScale(it)) },
            )
            SettingSlider(
                label = "Line height",
                valueLabel = String.format("%.2f", settings.lineHeight),
                value = settings.lineHeight,
                range = 1.25f..2f,
                onValueChange = { onAction(ReaderAction.SetLineHeight(it)) },
            )
            SettingSlider(
                label = "Margins",
                valueLabel = "${settings.horizontalMargin} dp",
                value = settings.horizontalMargin.toFloat(),
                range = 16f..48f,
                onValueChange = { onAction(ReaderAction.SetMargin(it.roundToInt())) },
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    SettingLabel("Show reading progress")
                    Text(
                        "Chapter and overall position",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(
                    checked = settings.showProgress,
                    onCheckedChange = { onAction(ReaderAction.SetProgressVisible(it)) },
                )
            }
        }
    }
}

@Composable
private fun SettingLabel(label: String) {
    Text(
        text = label,
        style = MaterialTheme.typography.titleMedium,
    )
}

@Composable
private fun <T> ChoiceRow(
    choices: List<T>,
    selected: T,
    label: (T) -> String,
    onSelected: (T) -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 10.dp),
        horizontalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        choices.forEach { choice ->
            TextButton(
                onClick = { onSelected(choice) },
                modifier = Modifier.weight(1f),
                colors = ButtonDefaults.textButtonColors(
                    containerColor = if (choice == selected) MaterialTheme.colorScheme.primary else Color.Transparent,
                    contentColor = if (choice == selected) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface,
                ),
                shape = RoundedCornerShape(10.dp),
            ) {
                Text(
                    text = label(choice),
                    maxLines = 1,
                    fontSize = 12.sp,
                )
            }
        }
    }
}

@Composable
private fun SettingSlider(
    label: String,
    valueLabel: String,
    value: Float,
    range: ClosedFloatingPointRange<Float>,
    onValueChange: (Float) -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        SettingLabel(label)
        Text(
            valueLabel,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
    Slider(
        value = value,
        onValueChange = onValueChange,
        valueRange = range,
    )
}
