package dev.bookharbor.app.notify

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.SharedPreferences
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import dev.bookharbor.app.AppGraph
import dev.bookharbor.app.MainActivity
import dev.bookharbor.app.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/** What the server shows now, reduced to the IDs that notifications care about. */
data class Snapshot(val bookTitles: Map<String, String>, val fulfilledRequests: Map<String, String>, val incomingFriends: Map<String, String>)

/** One notification to post: a stable [id] per kind, so a newer one replaces the older. */
data class Alert(val id: Int, val title: String, val text: String)

/**
 * The alerts for what appeared since [seen]: new books, fulfilled requests, and friend
 * requests. With nothing seen yet (a fresh sign-in) there is nothing "new" to announce.
 */
fun alertsFor(seen: Snapshot?, now: Snapshot): List<Alert> {
    if (seen == null) return emptyList()
    val alerts = ArrayList<Alert>()
    val books = now.bookTitles.filterKeys { it !in seen.bookTitles }.values.toList()
    if (books.isNotEmpty()) alerts += Alert(1, if (books.size == 1) "New in your library" else "${books.size} new books in your library", books.take(3).joinToString(", ") + if (books.size > 3) "…" else "")
    val fulfilled = now.fulfilledRequests.filterKeys { it !in seen.fulfilledRequests }.values.toList()
    if (fulfilled.isNotEmpty()) alerts += Alert(2, if (fulfilled.size == 1) "Your request is ready" else "${fulfilled.size} requests are ready", fulfilled.joinToString(", ") + " " + (if (fulfilled.size == 1) "is" else "are") + " now in the library")
    val friends = now.incomingFriends.filterKeys { it !in seen.incomingFriends }.values.toList()
    if (friends.isNotEmpty()) alerts += Alert(3, "Friend request", friends.joinToString(", ") + (if (friends.size == 1) " wants" else " want") + " to be friends")
    return alerts
}

/** Checks the server every few hours and posts a notification for anything new. */
class LibraryCheckWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val graph = AppGraph.get(applicationContext)
        if (graph.session.tokens == null || !Notifications.enabled(graph.prefs)) return@withContext Result.success()
        val now = try {
            Snapshot(
                bookTitles = graph.library.books().associate { it.id to it.title },
                fulfilledRequests = graph.bookRequests.mine().filter { it.status == "fulfilled" }.associate { it.id to it.title },
                incomingFriends = graph.friends.requests().incoming.associate { it.userId to it.displayName },
            )
        } catch (_: Exception) {
            return@withContext Result.retry()
        }
        val seen = Notifications.seen(graph.prefs)
        Notifications.remember(graph.prefs, now)
        alertsFor(seen, now).forEach { Notifications.post(applicationContext, it) }
        Result.success()
    }
}

object Notifications {
    private const val CHANNEL = "library"
    private const val ENABLED = "notify.enabled"
    private const val SEEN = "notify.seen"
    private const val WORK = "library-check"

    fun enabled(prefs: SharedPreferences) = prefs.getBoolean(ENABLED, true)

    fun setEnabled(context: Context, prefs: SharedPreferences, enabled: Boolean) {
        prefs.edit().putBoolean(ENABLED, enabled).apply()
        if (enabled) schedule(context) else WorkManager.getInstance(context).cancelUniqueWork(WORK)
    }

    /** Every 6 hours with a connection; the platform batches it with other apps' work. */
    fun schedule(context: Context) {
        val request = PeriodicWorkRequestBuilder<LibraryCheckWorker>(6, TimeUnit.HOURS)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(WORK, ExistingPeriodicWorkPolicy.KEEP, request)
    }

    /** Signing out forgets what this account had seen, so the next account starts fresh. */
    fun forget(prefs: SharedPreferences) { prefs.edit().remove(SEEN).apply() }

    internal fun seen(prefs: SharedPreferences): Snapshot? = prefs.getString(SEEN, null)?.let { decode(it) }

    internal fun remember(prefs: SharedPreferences, snapshot: Snapshot) { prefs.edit().putString(SEEN, encode(snapshot)).apply() }

    fun post(context: Context, alert: Alert) {
        if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) return
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "Library updates", NotificationManager.IMPORTANCE_DEFAULT).apply {
            description = "New books, fulfilled requests, and friend requests"
        })
        val open = PendingIntent.getActivity(context, alert.id, Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val notification = NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(R.drawable.ic_notification)
            .setColor(0xFF236059.toInt()) // SeaTeal
            .setContentTitle(alert.title)
            .setContentText(alert.text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(alert.text))
            .setContentIntent(open)
            .setAutoCancel(true)
            .build()
        runCatching { NotificationManagerCompat.from(context).notify(alert.id, notification) }
    }

    private fun encode(snapshot: Snapshot): String = org.json.JSONObject().apply {
        put("books", org.json.JSONObject(snapshot.bookTitles))
        put("requests", org.json.JSONObject(snapshot.fulfilledRequests))
        put("friends", org.json.JSONObject(snapshot.incomingFriends))
    }.toString()

    private fun decode(json: String): Snapshot? = runCatching {
        val root = org.json.JSONObject(json)
        fun map(key: String) = root.optJSONObject(key)?.let { o -> o.keys().asSequence().associateWith { o.getString(it) } }.orEmpty()
        Snapshot(map("books"), map("requests"), map("friends"))
    }.getOrNull()
}
