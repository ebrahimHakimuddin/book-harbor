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

    /** This reader's pace in EPUB chapter bytes per minute, learned from their own reading. */
    fun bytesPerMinute(): Double = preferences.getFloat(PACE_KEY, DEFAULT_BYTES_PER_MINUTE.toFloat()).toDouble()

    /** Folds in [bytes] read over [seconds] of steady reading; implausible samples are ignored. */
    fun recordPace(bytes: Double, seconds: Double) {
        val blended = blendPace(bytesPerMinute(), bytes, seconds) ?: return
        preferences.edit().putFloat(PACE_KEY, blended.toFloat()).apply()
    }

    private companion object {
        const val PREFIX = "stats.day."
        const val MIN_SECONDS = 60L
        const val PACE_KEY = "stats.pace"
    }
}

/** ~250 words a minute, in XHTML bytes (text plus typical markup). */
internal const val DEFAULT_BYTES_PER_MINUTE = 2000.0

/**
 * A new pace estimate from [current] and one sample, or null to ignore the sample: too short to
 * mean anything, or too fast to be reading (a jump or a skim).
 */
internal fun blendPace(current: Double, bytes: Double, seconds: Double): Double? {
    if (seconds < 20 || bytes <= 0) return null
    val sample = bytes / (seconds / 60)
    if (sample !in 300.0..12_000.0) return null
    return current * 0.8 + sample * 0.2
}

/** "3 min left in chapter", "1 h 10 min left in chapter", or null for less than a minute. */
fun minutesLeftLabel(bytesLeft: Double, bytesPerMinute: Double): String? {
    val minutes = kotlin.math.ceil(bytesLeft / bytesPerMinute.coerceAtLeast(1.0)).toInt()
    return when {
        minutes < 1 -> null
        minutes < 60 -> "$minutes min left in chapter"
        else -> "${minutes / 60} h ${minutes % 60} min left in chapter"
    }
}

/** Consecutive reading days ending today -- or yesterday, so a streak isn't "lost" before today's session. */
internal fun streak(days: Set<LocalDate>, today: LocalDate): Int {
    var day = if (today in days) today else today.minusDays(1)
    var count = 0
    while (day in days) { count++; day = day.minusDays(1) }
    return count
}
