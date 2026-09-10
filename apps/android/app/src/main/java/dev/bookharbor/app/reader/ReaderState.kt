package dev.bookharbor.app.reader

import kotlin.math.roundToInt

enum class ReaderTheme(val label: String) {
    System("System"),
    Light("Light"),
    Sepia("Sepia"),
    Dark("Dark"),
    Black("Black"),
}

enum class ReaderTypeface(val label: String) {
    Publisher("Publisher"),
    Literata("Literata"),
    Inter("Inter"),
}

data class ReaderSettings(
    val theme: ReaderTheme = ReaderTheme.System,
    val typeface: ReaderTypeface = ReaderTypeface.Literata,
    val fontScale: Float = 1f,
    val lineHeight: Float = 1.55f,
    val horizontalMargin: Int = 24,
    val showProgress: Boolean = true,
)

data class ReaderChapter(
    val id: String,
    val title: String,
    val paragraphs: List<String>,
)

data class ReaderState(
    val bookTitle: String,
    val chapters: List<ReaderChapter>,
    val chapterIndex: Int = 0,
    val chapterProgress: Map<String, Float> = emptyMap(),
    val settings: ReaderSettings = ReaderSettings(),
    val settingsOpen: Boolean = false,
) {
    init {
        require(chapters.isNotEmpty())
        require(chapterIndex in chapters.indices)
    }

    val chapter: ReaderChapter get() = chapters[chapterIndex]
    val nextChapter: ReaderChapter? get() = chapters.getOrNull(chapterIndex + 1)
    val currentChapterProgress: Float get() = chapterProgress[chapter.id] ?: 0f
    val overallProgress: Float get() = (chapterIndex + currentChapterProgress) / chapters.size
    val overallPercentage: Int get() = (overallProgress * 100).roundToInt()

    companion object {
        fun preview() = ReaderState(
            bookTitle = "The Cartographer's Wake",
            chapters = listOf(
                ReaderChapter(
                    id = "chapter-1",
                    title = "The Map Room",
                    paragraphs = previewParagraphs,
                ),
                ReaderChapter(
                    id = "chapter-2",
                    title = "The Sounding Line",
                    paragraphs = previewParagraphs.reversed(),
                ),
                ReaderChapter(
                    id = "chapter-3",
                    title = "Where the Water Remembers",
                    paragraphs = previewParagraphs,
                ),
            ),
        )
    }
}

sealed interface ReaderAction {
    data class RecordProgress(val fraction: Float) : ReaderAction
    data object NextChapter : ReaderAction
    data object OpenSettings : ReaderAction
    data object CloseSettings : ReaderAction
    data class SelectTheme(val theme: ReaderTheme) : ReaderAction
    data class SelectTypeface(val typeface: ReaderTypeface) : ReaderAction
    data class SetFontScale(val scale: Float) : ReaderAction
    data class SetLineHeight(val scale: Float) : ReaderAction
    data class SetMargin(val dp: Int) : ReaderAction
    data class SetProgressVisible(val visible: Boolean) : ReaderAction
}

fun ReaderState.reduce(action: ReaderAction): ReaderState = when (action) {
    is ReaderAction.RecordProgress -> copy(
        chapterProgress = chapterProgress + (chapter.id to action.fraction.coerceIn(0f, 1f)),
    )
    ReaderAction.NextChapter -> if (nextChapter == null) {
        copy(chapterProgress = chapterProgress + (chapter.id to 1f))
    } else {
        copy(
            chapterIndex = chapterIndex + 1,
            chapterProgress = chapterProgress + (chapter.id to 1f),
        )
    }
    ReaderAction.OpenSettings -> copy(settingsOpen = true)
    ReaderAction.CloseSettings -> copy(settingsOpen = false)
    is ReaderAction.SelectTheme -> copy(settings = settings.copy(theme = action.theme))
    is ReaderAction.SelectTypeface -> copy(settings = settings.copy(typeface = action.typeface))
    is ReaderAction.SetFontScale -> copy(settings = settings.copy(fontScale = action.scale.coerceIn(0.8f, 1.5f)))
    is ReaderAction.SetLineHeight -> copy(settings = settings.copy(lineHeight = action.scale.coerceIn(1.25f, 2f)))
    is ReaderAction.SetMargin -> copy(settings = settings.copy(horizontalMargin = action.dp.coerceIn(16, 48)))
    is ReaderAction.SetProgressVisible -> copy(settings = settings.copy(showProgress = action.visible))
}

private val previewParagraphs = listOf(
    "At first light, Mara found the harbor exactly where the old chart said it would not be. The breakwater curved from the mist like a sentence revised in the night, every stone wet with a pale and patient shine.",
    "She unfolded the map across the wheelhouse table. Its paper had softened along the creases, but the ink remained stubborn: a coast running north, three islands set like dark commas, and beyond them an empty field where the harbor now waited.",
    "The sounding bell moved somewhere below deck. Each note traveled through the hull before reaching the air, and for a moment the boat seemed less built than remembered—timber, rope, and brass assembled by the water's attention.",
    "On shore, the lamps were going out one by one. A figure in a blue coat stood at the end of the pier with both hands in his pockets. He did not wave. He only watched the boat approach, as though measuring it against a promise made years ago.",
    "Mara drew a clean line through the printed coastline. Then she marked the harbor in the margin, writing small enough to leave room for whatever else the morning might reveal.",
)
