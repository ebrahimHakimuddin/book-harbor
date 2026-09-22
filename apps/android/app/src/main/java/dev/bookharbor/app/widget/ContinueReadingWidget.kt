package dev.bookharbor.app.widget

import android.app.PendingIntent
import android.appwidget.AppWidgetManager
import android.appwidget.AppWidgetProvider
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.view.View
import android.widget.RemoteViews
import dev.bookharbor.app.AppGraph
import dev.bookharbor.app.MainActivity
import dev.bookharbor.app.R
import dev.bookharbor.app.library.CatalogCache
import dev.bookharbor.app.library.CoverLoader
import dev.bookharbor.app.library.continueReading
import kotlin.concurrent.thread
import kotlin.math.roundToInt

/**
 * Home-screen "Continue reading" card. It reads only on-device state (the saved catalog, local
 * positions, cached covers), so it is right offline; tapping it opens that book directly.
 */
class ContinueReadingWidget : AppWidgetProvider() {
    override fun onUpdate(context: Context, manager: AppWidgetManager, ids: IntArray) {
        val pending = goAsync()
        thread { try { render(context) } finally { pending.finish() } }
    }

    companion object {
        const val EXTRA_BOOK_ID = "dev.bookharbor.app.OPEN_BOOK_ID"

        /** Redraws every placed widget from current local state. Safe to call from any thread. */
        fun refresh(context: Context) {
            thread { runCatching { render(context.applicationContext) } }
        }

        private fun render(context: Context) {
            val manager = AppWidgetManager.getInstance(context)
            val ids = manager.getAppWidgetIds(ComponentName(context, ContinueReadingWidget::class.java))
            if (ids.isEmpty()) return
            val graph = AppGraph.get(context)
            val books = if (graph.session.tokens == null) emptyList() else CatalogCache(graph.prefs).load()?.second.orEmpty()
            val positions = graph.progress.positions()
            val progress = positions.associate { it.bookId to it.percentage }
            val book = continueReading(books, progress, positions.associate { it.bookId to it.occurredAt })

            val views = RemoteViews(context.packageName, R.layout.widget_continue_reading)
            val open = Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            if (book == null) {
                views.setTextViewText(R.id.widget_title, context.getString(R.string.widget_empty))
                views.setViewVisibility(R.id.widget_progress_row, View.GONE)
                views.setImageViewResource(R.id.widget_cover, R.drawable.brand_mark)
            } else {
                val percent = ((progress[book.id] ?: 0.0) * 100).roundToInt()
                views.setTextViewText(R.id.widget_title, book.title)
                views.setViewVisibility(R.id.widget_progress_row, View.VISIBLE)
                views.setProgressBar(R.id.widget_progress, 100, percent, false)
                views.setTextViewText(R.id.widget_percent, "$percent%")
                CoverLoader(graph.api, graph.cacheDir).cached(book)?.let { views.setImageViewBitmap(R.id.widget_cover, it) }
                    ?: views.setImageViewResource(R.id.widget_cover, R.drawable.brand_mark)
                views.setContentDescription(R.id.widget_root, "Continue reading ${book.title}, $percent percent")
                open.putExtra(EXTRA_BOOK_ID, book.id)
            }
            views.setOnClickPendingIntent(R.id.widget_root, PendingIntent.getActivity(context, 0, open, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            manager.updateAppWidget(ids, views)
        }
    }
}
