package dev.bookharbor.app.reader

import kotlin.math.roundToInt

/** A chapter as the reader chrome sees it. [weight] is its share of the book's text. */
data class ReaderChapter(
    val id: String,
    val title: String,
    val weight: Long = 1,
)

data class ReaderState(
    val bookTitle: String,
    val chapters: List<ReaderChapter>,
    val chapterIndex: Int = 0,
    val chapterProgress: Map<String, Float> = emptyMap(),
    val settings: ReaderSettings = ReaderSettings(),
    val settingsOpen: Boolean = false,
    val contentsOpen: Boolean = false,
) {
    init {
        require(chapters.isNotEmpty())
        require(chapterIndex in chapters.indices)
    }

    val chapter: ReaderChapter get() = chapters[chapterIndex]
    val nextChapter: ReaderChapter? get() = chapters.getOrNull(chapterIndex + 1)
    val currentChapterProgress: Float get() = chapterProgress[chapter.id] ?: 0f

    /** Weighted by chapter size, so a short preface does not count as much as a long chapter. */
    val overallProgress: Float
        get() {
            val total = chapters.sumOf { it.weight }.coerceAtLeast(1).toDouble()
            val before = chapters.take(chapterIndex).sumOf { it.weight }
            return ((before + chapter.weight * currentChapterProgress.toDouble()) / total).toFloat().coerceIn(0f, 1f)
        }
    val overallPercentage: Int get() = (overallProgress * 100).roundToInt()

    companion object {
        fun preview() = ReaderState(
            bookTitle = "The Cartographer's Wake",
            chapters = listOf(
                ReaderChapter("chapter-1", "The Map Room"),
                ReaderChapter("chapter-2", "The Sounding Line"),
                ReaderChapter("chapter-3", "Where the Water Remembers"),
            ),
        )
    }
}

sealed interface ReaderAction {
    data class RecordProgress(val fraction: Float) : ReaderAction
    data object NextChapter : ReaderAction
    data class SelectChapter(val index: Int) : ReaderAction
    data object OpenSettings : ReaderAction
    data object CloseSettings : ReaderAction
    data object OpenContents : ReaderAction
    data object CloseContents : ReaderAction
    data class SelectTheme(val theme: ReaderTheme) : ReaderAction
    data class SelectTypeface(val typeface: ReaderTypeface) : ReaderAction
    data class SelectAlignment(val alignment: ReaderAlignment) : ReaderAction
    data class SetFontScale(val scale: Float) : ReaderAction
    data class SetLineHeight(val scale: Float) : ReaderAction
    data class SetParagraphSpacing(val scale: Float) : ReaderAction
    data class SetMargin(val dp: Int) : ReaderAction
    data class SetBrightness(val brightness: Float?) : ReaderAction
    data class SetProgressVisible(val visible: Boolean) : ReaderAction
    data class SetNightSchedule(val enabled: Boolean) : ReaderAction
    data class SetNightHours(val start: Int, val end: Int) : ReaderAction
    data class SetNightBrightness(val brightness: Float) : ReaderAction
    data class SetVolumeKeys(val enabled: Boolean) : ReaderAction
    data class SetOrientation(val orientation: ReaderOrientation) : ReaderAction
    data class SetHyphenation(val enabled: Boolean) : ReaderAction
    data class SetWordEmphasis(val enabled: Boolean) : ReaderAction
    data class SetLetterSpacing(val em: Float) : ReaderAction
    data class SetWordSpacing(val em: Float) : ReaderAction
    data object ResetSettings : ReaderAction
}

fun ReaderState.reduce(action: ReaderAction): ReaderState = when (action) {
    is ReaderAction.RecordProgress -> copy(chapterProgress = chapterProgress + (chapter.id to action.fraction.coerceIn(0f, 1f)))
    ReaderAction.NextChapter -> if (nextChapter == null) {
        copy(chapterProgress = chapterProgress + (chapter.id to 1f))
    } else {
        copy(chapterIndex = chapterIndex + 1, chapterProgress = chapterProgress + (chapter.id to 1f))
    }
    is ReaderAction.SelectChapter -> if (action.index in chapters.indices) copy(chapterIndex = action.index, contentsOpen = false) else this
    ReaderAction.OpenSettings -> copy(settingsOpen = true)
    ReaderAction.CloseSettings -> copy(settingsOpen = false)
    ReaderAction.OpenContents -> copy(contentsOpen = true)
    ReaderAction.CloseContents -> copy(contentsOpen = false)
    is ReaderAction.SelectTheme -> copy(settings = settings.copy(theme = action.theme))
    is ReaderAction.SelectTypeface -> copy(settings = settings.copy(typeface = action.typeface))
    is ReaderAction.SelectAlignment -> copy(settings = settings.copy(alignment = action.alignment))
    is ReaderAction.SetFontScale -> copy(settings = settings.copy(fontScale = action.scale.coerceIn(0.8f, 1.5f)))
    is ReaderAction.SetLineHeight -> copy(settings = settings.copy(lineHeight = action.scale.coerceIn(1.25f, 2f)))
    is ReaderAction.SetParagraphSpacing -> copy(settings = settings.copy(paragraphSpacing = action.scale.coerceIn(0.5f, 2f)))
    is ReaderAction.SetMargin -> copy(settings = settings.copy(horizontalMargin = action.dp.coerceIn(16, 48)))
    is ReaderAction.SetBrightness -> copy(settings = settings.copy(brightness = action.brightness?.coerceIn(0.05f, 1f)))
    is ReaderAction.SetProgressVisible -> copy(settings = settings.copy(showProgress = action.visible))
    is ReaderAction.SetNightSchedule -> copy(settings = settings.copy(nightSchedule = action.enabled))
    is ReaderAction.SetNightHours -> copy(settings = settings.copy(nightStart = action.start.coerceIn(0, 23), nightEnd = action.end.coerceIn(0, 23)))
    is ReaderAction.SetNightBrightness -> copy(settings = settings.copy(nightBrightness = action.brightness.coerceIn(0.05f, 1f)))
    is ReaderAction.SetVolumeKeys -> copy(settings = settings.copy(volumeKeys = action.enabled))
    is ReaderAction.SetOrientation -> copy(settings = settings.copy(orientation = action.orientation))
    is ReaderAction.SetHyphenation -> copy(settings = settings.copy(hyphenation = action.enabled))
    is ReaderAction.SetWordEmphasis -> copy(settings = settings.copy(wordEmphasis = action.enabled))
    is ReaderAction.SetLetterSpacing -> copy(settings = settings.copy(letterSpacing = action.em.coerceIn(0f, 0.15f)))
    is ReaderAction.SetWordSpacing -> copy(settings = settings.copy(wordSpacing = action.em.coerceIn(0f, 0.6f)))
    ReaderAction.ResetSettings -> copy(settings = ReaderSettings.Default)
}
