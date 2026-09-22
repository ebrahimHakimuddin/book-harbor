package dev.bookharbor.app.ui

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.PathParser
import androidx.compose.ui.unit.dp

/** Outline icons in the same 1.75px round-cap style as the web console. Tint them with `Icon(tint = …)`. */
object BrandIcons {
    private fun icon(name: String, vararg paths: String): ImageVector = ImageVector.Builder(name, 24.dp, 24.dp, 24f, 24f).apply {
        paths.forEach { data ->
            addPath(
                pathData = PathParser().parsePathString(data).toNodes(),
                stroke = SolidColor(Color.Black),
                strokeLineWidth = 1.75f,
                strokeLineCap = StrokeCap.Round,
                strokeLineJoin = StrokeJoin.Round,
            )
        }
    }.build()

    private const val CLOUD = "M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9z"

    val Search = icon("search", "M11 3a8 8 0 1 0 0 16 8 8 0 0 0 0-16z", "M21 21l-4.3-4.3")
    val Filter = icon("filter", "M21 4h-7M10 4H3M21 12h-9M8 12H3M21 20h-5M12 20H3M14 2v4M8 10v4M16 18v4")
    val Cloud = icon("cloud", CLOUD)
    val CloudDone = icon("cloud-done", CLOUD, "M9.5 13.5l2 2 3.5-3.5")
    val CloudOff = icon("cloud-off", "M2 2l20 20", "M5.8 5.8A7 7 0 0 0 9 19h8.5a4.5 4.5 0 0 0 1.3-.2", "M21.5 16.5A4.5 4.5 0 0 0 17.5 10h-1.8A7 7 0 0 0 10 5.1")
    val Library = icon("library", "M2 4.5h6a4 4 0 0 1 4 4V20a3 3 0 0 0-3-3H2z", "M22 4.5h-6a4 4 0 0 0-4 4V20a3 3 0 0 1 3-3h7z")
    val Sync = icon("sync", "M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8", "M21 3v5h-5", "M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16", "M8 16H3v5")
    val More = icon("more", "M5 12h.01M12 12h.01M19 12h.01")
    val MoreVertical = icon("more-vertical", "M12 5h.01M12 12h.01M12 19h.01")
    val Download = icon("download", "M12 4v11m0 0-4-4m4 4 4-4", "M4 20h16")
    val Check = icon("check", "M5 12.5l4.5 4.5L19 7")
    val Trash = icon("trash", "M4 7h16M9 7V4.5h6V7M6 7l1 13h10l1-13M10 11v6M14 11v6")
    val Close = icon("close", "M6 6l12 12M18 6L6 18")
    val SignOut = icon("sign-out", "M9 4H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h3", "M16 8l4 4-4 4M20 12H9")
    val Server = icon("server", "M4 4h16a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z", "M4 13h16a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-5a1 1 0 0 1 1-1z", "M7 7.5h.01M7 16.5h.01")
    val Friends = icon("friends", "M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2", "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8", "M23 21v-2a4 4 0 0 0-3-3.87", "M16 3.13a4 4 0 0 1 0 7.75")
    val History = icon("history", "M3 12a9 9 0 1 0 2.6-6.3", "M3 4v5h5", "M12 8v4l3 2")
    val ChevronLeft = icon("chevron-left", "M15 5l-7 7 7 7")
    val ChevronRight = icon("chevron-right", "M9 5l7 7-7 7")
    val List = icon("list", "M8 6h13M8 12h13M8 18h13", "M3 6h.01M3 12h.01M3 18h.01")
    val Grid = icon("grid", "M3 3h8v8H3z", "M13 3h8v8h-8z", "M3 13h8v8H3z", "M13 13h8v8h-8z")
    val Bookmark = icon("bookmark", "M6 3h12v18l-6-4.5L6 21z")
    val BookmarkAdded = icon("bookmark-added", "M6 3h12v18l-6-4.5L6 21z", "M9.5 9.5l2 2 3.5-3.5")
    val Share = icon("share", "M18 8a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM6 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM18 22a3 3 0 1 0 0-6 3 3 0 0 0 0 6z", "M8.6 13.5l6.8 4M15.4 6.5l-6.8 4")
    val Request = icon("request", "M4 19.5A2.5 2.5 0 0 1 6.5 17H20", "M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z", "M9 7h7M9 10h4")
}
