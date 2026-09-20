package dev.bookharbor.app.library

import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

const val GITHUB_URL = "https://github.com/ebrahimHakimuddin/book-harbor"

/**
 * A newer release than the installed one, or null when up to date. Throws when GitHub can't be
 * reached. [downloadUrl] is the release's APK asset, so opening it starts a file download
 * directly rather than landing on the GitHub releases page; it falls back to that page only if
 * a release is somehow missing its APK.
 */
data class Release(val version: String, val downloadUrl: String)

fun latestRelease(): Release {
    val connection = URL("https://api.github.com/repos/ebrahimHakimuddin/book-harbor/releases/latest").openConnection() as HttpURLConnection
    try {
        connection.connectTimeout = 10_000
        connection.readTimeout = 10_000
        connection.setRequestProperty("Accept", "application/vnd.github+json")
        if (connection.responseCode != 200) throw java.io.IOException("GitHub returned ${connection.responseCode}")
        val json = JSONObject(connection.inputStream.bufferedReader().readText())
        val assets = json.optJSONArray("assets") ?: org.json.JSONArray()
        val apkUrl = (0 until assets.length()).map { assets.getJSONObject(it) }
            .firstOrNull { it.getString("name").endsWith(".apk") }
            ?.getString("browser_download_url")
        return Release(json.getString("tag_name").removePrefix("v"), apkUrl ?: json.getString("html_url"))
    } finally {
        connection.disconnect()
    }
}

/** True when [candidate] is a higher dotted version than [installed] ("0.10.0" > "0.9.1"). */
fun isNewer(candidate: String, installed: String): Boolean {
    val a = candidate.split('.').map { it.toIntOrNull() ?: 0 }
    val b = installed.split('.').map { it.toIntOrNull() ?: 0 }
    for (i in 0 until maxOf(a.size, b.size)) {
        val x = a.getOrElse(i) { 0 }; val y = b.getOrElse(i) { 0 }
        if (x != y) return x > y
    }
    return false
}
