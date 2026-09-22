package dev.bookharbor.app.ui.theme

import android.app.Activity
import android.content.res.Configuration
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.SideEffect
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat
import dev.bookharbor.app.R
import dev.bookharbor.app.reader.ReaderTheme

val HarborNavy = Color(0xFF0F2D46)
// Darkened from the original F4A261-paired 0xFF2E7D7A: that shade only cleared 4.48:1 against
// Sand, just under the 4.5:1 body-text minimum. This clears 6.7:1+ against every light/sepia surface.
val SeaTeal = Color(0xFF236059)
val MistCyan = Color(0xFF7FB3C3)
val Sand = Color(0xFFF8F6EF)
val Sunrise = Color(0xFFF4A261)
/** A darker amber for light/sepia surfaces, where raw [Sunrise] only clears ~2:1 contrast as text. */
private val CautionAmberLight = Color(0xFF9C5211)

/**
 * Both faces ship as single variable-font files. Each weight must set the file's `wght` axis
 * itself: a Font declared Bold is trusted to already be bold, so without the variation setting
 * every weight rendered at the file's default 400 and nothing in the app was ever bold.
 */
@OptIn(ExperimentalTextApi::class)
private fun variableFamily(resource: Int) = FontFamily(
    listOf(FontWeight.Normal, FontWeight.Medium, FontWeight.SemiBold, FontWeight.Bold).map { weight ->
        Font(resource, weight = weight, variationSettings = FontVariation.Settings(FontVariation.weight(weight.weight)))
    },
)

val InterFamily = variableFamily(R.font.inter)

val LiterataFamily = variableFamily(R.font.literata)

/**
 * The full type scale -- every Material style is defined, so nothing silently falls back to
 * Roboto. Literata carries headings, Inter the interface. Tracking tightens as size grows and
 * opens slightly on small labels; leading is tight on display sizes and roomy on body text.
 */
private val BrandTypography = androidx.compose.material3.Typography(
    displayLarge = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 48.sp, lineHeight = 52.sp, letterSpacing = (-0.8).sp),
    displayMedium = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 40.sp, lineHeight = 44.sp, letterSpacing = (-0.6).sp),
    displaySmall = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 34.sp, lineHeight = 40.sp, letterSpacing = (-0.5).sp),
    headlineLarge = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 32.sp, lineHeight = 38.sp, letterSpacing = (-0.5).sp),
    headlineMedium = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 26.sp, lineHeight = 32.sp, letterSpacing = (-0.3).sp),
    headlineSmall = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 23.sp, lineHeight = 29.sp, letterSpacing = (-0.2).sp),
    titleLarge = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 21.sp, lineHeight = 27.sp, letterSpacing = (-0.1).sp),
    titleMedium = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.SemiBold, fontSize = 16.sp, lineHeight = 22.sp, letterSpacing = (-0.1).sp),
    titleSmall = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.SemiBold, fontSize = 14.sp, lineHeight = 20.sp),
    bodyLarge = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Normal, fontSize = 17.sp, lineHeight = 26.sp),
    bodyMedium = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Normal, fontSize = 15.sp, lineHeight = 22.sp),
    bodySmall = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Normal, fontSize = 13.sp, lineHeight = 18.sp, letterSpacing = 0.1.sp),
    labelLarge = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.SemiBold, fontSize = 14.sp, lineHeight = 20.sp),
    labelMedium = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Medium, fontSize = 12.sp, lineHeight = 17.sp, letterSpacing = 0.2.sp),
    labelSmall = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Medium, fontSize = 11.sp, lineHeight = 15.sp, letterSpacing = 0.3.sp),
)

/**
 * Fills every color role the schemes below don't set by hand, from the brand colors. Material 3
 * reads many roles we never named -- outlineVariant for outlined-button borders, secondaryContainer
 * for slider tracks, the surfaceContainer family for switches, menus, and sheets -- and any role
 * left unset falls back to Material's purple baseline, which is what leaked into those borders.
 */
private fun ColorScheme.branded() = copy(
    primaryContainer = lerp(surface, primary, 0.22f), onPrimaryContainer = onSurface,
    secondaryContainer = lerp(surface, secondary, 0.28f), onSecondaryContainer = onSurface,
    tertiaryContainer = lerp(surface, tertiary, 0.28f), onTertiaryContainer = onSurface,
    outlineVariant = lerp(surface, outline, 0.5f),
    surfaceTint = secondary,
    surfaceContainerLowest = background,
    surfaceContainerLow = lerp(background, surface, 0.5f),
    surfaceContainer = surface,
    surfaceContainerHigh = lerp(surface, surfaceVariant, 0.5f),
    surfaceContainerHighest = surfaceVariant,
    surfaceBright = lerp(surface, surfaceVariant, 0.7f),
    surfaceDim = background,
    inverseSurface = onSurface, inverseOnSurface = surface, inversePrimary = secondary,
)

private val LightColors = lightColorScheme(
    primary = HarborNavy,
    onPrimary = Color.White,
    secondary = SeaTeal,
    onSecondary = Color.White,
    tertiary = Sunrise,
    background = Sand,
    onBackground = HarborNavy,
    surface = Color(0xFFFFFDF8),
    onSurface = HarborNavy,
    surfaceVariant = Color(0xFFE6EEF0),
    onSurfaceVariant = Color(0xFF425E6D),
    outline = MistCyan,
).branded()

private val SepiaColors = LightColors.copy(
    background = Color(0xFFF3E8D1),
    surface = Color(0xFFF8EEDB),
    surfaceVariant = Color(0xFFE8D8BA),
    onSurfaceVariant = Color(0xFF5D5445),
    outline = Color(0xFFA9A17E),
).branded()

private val DarkColors = darkColorScheme(
    primary = Color(0xFFAED8E3),
    onPrimary = Color(0xFF05283A),
    secondary = Color(0xFF76C8C3),
    onSecondary = Color(0xFF00201F),
    tertiary = Sunrise,
    background = Color(0xFF102735),
    onBackground = Color(0xFFE5F0F3),
    surface = Color(0xFF173442),
    onSurface = Color(0xFFE5F0F3),
    surfaceVariant = Color(0xFF294957),
    onSurfaceVariant = Color(0xFFC2D9E0),
    outline = Color(0xFF6F929E),
).branded()

private val BlackColors = DarkColors.copy(
    background = Color.Black,
    surface = Color(0xFF0D1113),
    surfaceVariant = Color(0xFF1B2428),
).branded()

/** The colors [theme] resolves to right now ("System" follows the device's dark mode). */
@Composable
fun readerColorScheme(theme: ReaderTheme): ColorScheme {
    val systemDark = LocalConfiguration.current.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
    return when (theme) {
        ReaderTheme.System -> if (systemDark) DarkColors else LightColors
        ReaderTheme.Light -> LightColors
        ReaderTheme.Sepia -> SepiaColors
        ReaderTheme.Dark -> DarkColors
        ReaderTheme.Black -> BlackColors
    }
}

@Composable
fun BookHarborTheme(
    readerTheme: ReaderTheme,
    content: @Composable () -> Unit,
) {
    val colors = readerColorScheme(readerTheme)
    ApplySystemBars(colors)
    MaterialTheme(
        colorScheme = colors,
        typography = BrandTypography,
        content = content,
    )
}

/**
 * A distinct color for a reversible/lower-severity destructive action (e.g. removing a
 * download you can refetch), separate from [MaterialTheme]'s `error` (a severe or
 * irreversible action, e.g. removing a friend or signing out with unsynced progress).
 * Material3's ColorScheme has no built-in "caution" role, so this is a plain function
 * rather than a ColorScheme field; it tracks the current background so it stays legible
 * across every reader theme.
 */
@Composable
fun cautionColor(): Color = if (MaterialTheme.colorScheme.background.luminance() > 0.5f) CautionAmberLight else Sunrise

@Composable
private fun ApplySystemBars(colors: ColorScheme) {
    val view = LocalView.current
    if (!view.isInEditMode) {
        SideEffect {
            val window = (view.context as Activity).window
            WindowCompat.getInsetsController(window, view).apply {
                isAppearanceLightStatusBars = colors.background.luminance() > 0.5f
                isAppearanceLightNavigationBars = colors.background.luminance() > 0.5f
            }
        }
    }
}
