package dev.bookharbor.app.reader

import android.content.SharedPreferences
import android.os.SystemClock
import java.time.LocalDate

/**
 * Time spent in the reader per local day, kept on the device. A session runs from [start] to
 * [stop] (the reader resuming and pausing) and is credited to the day it ends.
 */
class ReadingStats(private val preferences: SharedPreferences, private val today: () -> LocalDate = LocalDate::now) {
    private var sessionStart: Long? = null

    fun start() { sessionStart = SystemClock.elapsedRealtime() }

    fun stop() {
        val started = sessionStart ?: return
        sessionStart = null
        val seconds = (SystemClock.elapsedRealtime() - started) / 1000
        val key = PREFIX + today()
        preferences.edit().putLong(key, preferences.getLong(key, 0) + seconds).apply()
    }

    /** Includes the session in progress, so the figure is live while reading. */
    fun secondsToday(): Long =
        preferences.getLong(PREFIX + today(), 0) + (sessionStart?.let { (SystemClock.elapsedRealtime() - it) / 1000 } ?: 0)

    fun streakDays(): Int {
        val days = preferences.all.mapNotNull { (key, value) ->
            if (!key.startsWith(PREFIX) || (value as? Long ?: 0) < MIN_SECONDS) null
            else runCatching { LocalDate.parse(key.removePrefix(PREFIX)) }.getOrNull()
        }.toSet()
        return streak(days + listOfNotNull(today().takeIf { secondsToday() >= MIN_SECONDS }), today())
    }

    private companion object {
        const val PREFIX = "stats.day."
        const val MIN_SECONDS = 60L
    }
}

/** Consecutive reading days ending today -- or yesterday, so a streak isn't "lost" before today's session. */
internal fun streak(days: Set<LocalDate>, today: LocalDate): Int {
    var day = if (today in days) today else today.minusDays(1)
    var count = 0
    while (day in days) { count++; day = day.minusDays(1) }
    return count
}
