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
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import androidx.core.view.WindowCompat
import dev.bookharbor.app.R
import dev.bookharbor.app.reader.ReaderTheme

val HarborNavy = Color(0xFF0F2D46)
val SeaTeal = Color(0xFF2E7D7A)
val MistCyan = Color(0xFF7FB3C3)
val Sand = Color(0xFFF8F6EF)
val Sunrise = Color(0xFFF4A261)

val InterFamily = FontFamily(
    Font(R.font.inter, weight = FontWeight.Normal),
    Font(R.font.inter, weight = FontWeight.Medium),
    Font(R.font.inter, weight = FontWeight.SemiBold),
    Font(R.font.inter, weight = FontWeight.Bold),
)

val LiterataFamily = FontFamily(
    Font(R.font.literata, weight = FontWeight.Normal),
    Font(R.font.literata, weight = FontWeight.Medium),
    Font(R.font.literata, weight = FontWeight.SemiBold),
    Font(R.font.literata, weight = FontWeight.Bold),
)

private val BrandTypography = androidx.compose.material3.Typography(
    displayLarge = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 48.sp, lineHeight = 54.sp),
    headlineLarge = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 32.sp, lineHeight = 38.sp),
    headlineMedium = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 26.sp, lineHeight = 32.sp),
    titleLarge = TextStyle(fontFamily = LiterataFamily, fontWeight = FontWeight.SemiBold, fontSize = 21.sp, lineHeight = 27.sp),
    titleMedium = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.SemiBold, fontSize = 16.sp, lineHeight = 22.sp),
    bodyLarge = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Normal, fontSize = 17.sp, lineHeight = 26.sp),
    bodyMedium = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Normal, fontSize = 15.sp, lineHeight = 22.sp),
    labelLarge = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.SemiBold, fontSize = 14.sp, lineHeight = 20.sp),
    labelMedium = TextStyle(fontFamily = InterFamily, fontWeight = FontWeight.Medium, fontSize = 12.sp, lineHeight = 17.sp),
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
)

private val SepiaColors = LightColors.copy(
    background = Color(0xFFF3E8D1),
    surface = Color(0xFFF8EEDB),
    surfaceVariant = Color(0xFFE8D8BA),
    onSurfaceVariant = Color(0xFF5D5445),
    outline = Color(0xFFA9A17E),
)

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
)

private val BlackColors = DarkColors.copy(
    background = Color.Black,
    surface = Color(0xFF0D1113),
    surfaceVariant = Color(0xFF1B2428),
)

@Composable
fun BookHarborTheme(
    readerTheme: ReaderTheme,
    content: @Composable () -> Unit,
) {
    val systemDark = LocalConfiguration.current.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
    val colors = when (readerTheme) {
        ReaderTheme.System -> if (systemDark) DarkColors else LightColors
        ReaderTheme.Light -> LightColors
        ReaderTheme.Sepia -> SepiaColors
        ReaderTheme.Dark -> DarkColors
        ReaderTheme.Black -> BlackColors
    }
    ApplySystemBars(colors)
    MaterialTheme(
        colorScheme = colors,
        typography = BrandTypography,
        content = content,
    )
}

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
