package dev.bookharbor.app.reader

import android.content.SharedPreferences

enum class ReaderTheme(val label: String) {
    System("System"),
    Light("Light"),
    Sepia("Sepia"),
    Dark("Dark"),
    Black("Black"),
}

enum class ReaderTypeface(val label: String) {
    Literata("Literata"),
    Inter("Inter"),
}

enum class ReaderAlignment(val label: String) {
    Left("Left"),
    Justified("Justified"),
}

enum class ReaderOrientation(val label: String) {
    Auto("Auto"),
    Portrait("Portrait"),
    Landscape("Landscape"),
}

/** The reader profile. It lives on the device and applies to every book; no server round trip. */
data class ReaderSettings(
    val theme: ReaderTheme = ReaderTheme.System,
    val typeface: ReaderTypeface = ReaderTypeface.Literata,
    val fontScale: Float = 1f,
    val lineHeight: Float = 1.55f,
    val paragraphSpacing: Float = 1f,
    val horizontalMargin: Int = 24,
    val alignment: ReaderAlignment = ReaderAlignment.Left,
    /** 0.05..1, or null to follow the system brightness. */
    val brightness: Float? = null,
    val showProgress: Boolean = true,
    /** From [nightStart] to [nightEnd] o'clock, switch to a dark theme and dim to [nightBrightness]. */
    val nightSchedule: Boolean = false,
    val nightStart: Int = 21,
    val nightEnd: Int = 7,
    val nightBrightness: Float = 0.15f,
    /** Volume up/down scroll back/forward a screen. */
    val volumeKeys: Boolean = false,
    val orientation: ReaderOrientation = ReaderOrientation.Auto,
    /** Break long words at line ends, which keeps justified text from gapping. */
    val hyphenation: Boolean = true,
    /** Bold the opening letters of each word to guide the eye (the "bionic reading" technique). */
    val wordEmphasis: Boolean = false,
    /** Extra space between letters and after each word, in ems. */
    val letterSpacing: Float = 0f,
    val wordSpacing: Float = 0f,
) {
    /** True when [hour] (0..23) falls in the night window, which may wrap past midnight. */
    fun isNight(hour: Int): Boolean = when {
        !nightSchedule || nightStart == nightEnd -> false
        nightStart < nightEnd -> hour in nightStart until nightEnd
        else -> hour >= nightStart || hour < nightEnd
    }

    /** What actually applies at [hour]: the night schedule overrides theme and brightness. */
    fun effectiveAt(hour: Int): ReaderSettings {
        if (!isNight(hour)) return this
        return copy(
            theme = if (theme == ReaderTheme.Dark || theme == ReaderTheme.Black) theme else ReaderTheme.Black,
            brightness = minOf(brightness ?: nightBrightness, nightBrightness),
        )
    }

    fun toMap(): Map<String, String> = buildMap {
        put("theme", theme.name); put("typeface", typeface.name); put("fontScale", fontScale.toString())
        put("lineHeight", lineHeight.toString()); put("paragraphSpacing", paragraphSpacing.toString())
        put("margin", horizontalMargin.toString()); put("alignment", alignment.name)
        brightness?.let { put("brightness", it.toString()) }
        put("showProgress", showProgress.toString())
        put("nightSchedule", nightSchedule.toString())
        put("nightStart", nightStart.toString()); put("nightEnd", nightEnd.toString()); put("nightBrightness", nightBrightness.toString())
        put("volumeKeys", volumeKeys.toString()); put("orientation", orientation.name); put("hyphenation", hyphenation.toString())
        put("wordEmphasis", wordEmphasis.toString()); put("letterSpacing", letterSpacing.toString()); put("wordSpacing", wordSpacing.toString())
    }

    companion object {
        val Default = ReaderSettings()

        /** Tolerant of missing, renamed, or corrupt values: anything unreadable falls back to its default. */
        fun fromMap(map: Map<String, String?>): ReaderSettings = ReaderSettings(
            theme = enumOr(map["theme"], Default.theme),
            typeface = enumOr(map["typeface"], Default.typeface),
            fontScale = (map["fontScale"]?.toFloatOrNull() ?: Default.fontScale).coerceIn(0.8f, 1.5f),
            lineHeight = (map["lineHeight"]?.toFloatOrNull() ?: Default.lineHeight).coerceIn(1.25f, 2f),
            paragraphSpacing = (map["paragraphSpacing"]?.toFloatOrNull() ?: Default.paragraphSpacing).coerceIn(0.5f, 2f),
            horizontalMargin = (map["margin"]?.toIntOrNull() ?: Default.horizontalMargin).coerceIn(16, 48),
            alignment = enumOr(map["alignment"], Default.alignment),
            brightness = map["brightness"]?.toFloatOrNull()?.coerceIn(0.05f, 1f),
            showProgress = map["showProgress"]?.toBooleanStrictOrNull() ?: Default.showProgress,
            nightSchedule = map["nightSchedule"]?.toBooleanStrictOrNull() ?: Default.nightSchedule,
            nightStart = (map["nightStart"]?.toIntOrNull() ?: Default.nightStart).coerceIn(0, 23),
            nightEnd = (map["nightEnd"]?.toIntOrNull() ?: Default.nightEnd).coerceIn(0, 23),
            nightBrightness = (map["nightBrightness"]?.toFloatOrNull() ?: Default.nightBrightness).coerceIn(0.05f, 1f),
            volumeKeys = map["volumeKeys"]?.toBooleanStrictOrNull() ?: Default.volumeKeys,
            orientation = enumOr(map["orientation"], Default.orientation),
            hyphenation = map["hyphenation"]?.toBooleanStrictOrNull() ?: Default.hyphenation,
            wordEmphasis = map["wordEmphasis"]?.toBooleanStrictOrNull() ?: Default.wordEmphasis,
            letterSpacing = (map["letterSpacing"]?.toFloatOrNull() ?: Default.letterSpacing).coerceIn(0f, 0.15f),
            wordSpacing = (map["wordSpacing"]?.toFloatOrNull() ?: Default.wordSpacing).coerceIn(0f, 0.6f),
        )

        private inline fun <reified T : Enum<T>> enumOr(name: String?, fallback: T): T =
            enumValues<T>().firstOrNull { it.name == name } ?: fallback
    }
}

class ReaderSettingsStore(private val preferences: SharedPreferences) {
    fun load(): ReaderSettings = ReaderSettings.fromMap(KEYS.associateWith { preferences.getString(PREFIX + it, null) })

    fun save(settings: ReaderSettings) {
        preferences.edit().apply {
            KEYS.forEach { remove(PREFIX + it) } // clears brightness when it is back to "system"
            settings.toMap().forEach { (key, value) -> putString(PREFIX + key, value) }
        }.apply()
    }

    private companion object {
        const val PREFIX = "reader."
        val KEYS = listOf("theme", "typeface", "fontScale", "lineHeight", "paragraphSpacing", "margin", "alignment", "brightness", "showProgress", "nightSchedule", "nightStart", "nightEnd", "nightBrightness", "volumeKeys", "orientation", "hyphenation", "wordEmphasis", "letterSpacing", "wordSpacing")
    }
}
