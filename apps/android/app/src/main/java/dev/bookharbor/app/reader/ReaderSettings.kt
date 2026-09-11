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
    Publisher("Publisher"),
    Literata("Literata"),
    Inter("Inter"),
}

enum class ReaderAlignment(val label: String) {
    Publisher("Default"),
    Left("Left"),
    Justified("Justified"),
}

/** The reader profile. It lives on the device and applies to every book; no server round trip. */
data class ReaderSettings(
    val theme: ReaderTheme = ReaderTheme.System,
    val typeface: ReaderTypeface = ReaderTypeface.Literata,
    val fontScale: Float = 1f,
    val lineHeight: Float = 1.55f,
    val paragraphSpacing: Float = 1f,
    val horizontalMargin: Int = 24,
    val alignment: ReaderAlignment = ReaderAlignment.Publisher,
    /** 0.05..1, or null to follow the system brightness. */
    val brightness: Float? = null,
    val showProgress: Boolean = true,
) {
    fun toMap(): Map<String, String> = buildMap {
        put("theme", theme.name); put("typeface", typeface.name); put("fontScale", fontScale.toString())
        put("lineHeight", lineHeight.toString()); put("paragraphSpacing", paragraphSpacing.toString())
        put("margin", horizontalMargin.toString()); put("alignment", alignment.name)
        brightness?.let { put("brightness", it.toString()) }
        put("showProgress", showProgress.toString())
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
        val KEYS = listOf("theme", "typeface", "fontScale", "lineHeight", "paragraphSpacing", "margin", "alignment", "brightness", "showProgress")
    }
}
