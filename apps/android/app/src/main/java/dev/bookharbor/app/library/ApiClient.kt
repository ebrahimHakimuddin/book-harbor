package dev.bookharbor.app.library

import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.URL
import org.json.JSONObject

class HttpError(val status: Int, message: String) : Exception(message)

/** The server's error body is `{"code": "...", "message": "..."}`; fall back to the raw body if it isn't. */
private fun errorMessage(body: String): String = runCatching { JSONObject(body).getString("message") }.getOrDefault(body)

/**
 * The one place that talks HTTP to the BookHarbor server. Authorized calls send the
 * stored access token and, on a 401, refresh it once and retry.
 */
class ApiClient(
    val session: SessionStore,
    private val openConnection: (String) -> HttpURLConnection = { URL(it).openConnection() as HttpURLConnection },
    /** This app's version, sent on every request so the server can refuse a mismatched app. */
    private val clientVersion: String = "",
) {
    /** Called when the server refuses this app's version (426), with its explanation. */
    @Volatile var onVersionRejected: ((String) -> Unit)? = null

    /** Resolves a server-relative path (or returns an absolute URL unchanged). */
    fun url(path: String): String = URL(URL(session.serverUrl.trimEnd('/') + "/"), path).toString()

    fun requestBytes(
        url: String,
        method: String = "GET",
        body: String? = null,
        token: String? = null,
        readTimeoutMillis: Int = 15_000,
    ): ByteArray {
        val connection = openConnection(url)
        try {
            connection.requestMethod = method
            connection.connectTimeout = 8_000 // an unreachable home server should fail fast, not hang the UI
            connection.readTimeout = readTimeoutMillis
            connection.setRequestProperty("Accept", "application/json")
            if (clientVersion.isNotBlank()) connection.setRequestProperty("X-BookHarbor-Client", "android/$clientVersion")
            if (token != null) connection.setRequestProperty("Authorization", "Bearer $token")
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                connection.outputStream.use { it.write(body.toByteArray()) }
            }
            val status = connection.responseCode
            val stream = if (status in 200..299) connection.inputStream else connection.errorStream
            val bytes = stream?.use { input ->
                val out = ByteArrayOutputStream()
                input.copyTo(out)
                out.toByteArray()
            } ?: ByteArray(0)
            if (status == 426) onVersionRejected?.invoke(errorMessage(String(bytes)))
            if (status !in 200..299) throw HttpError(status, errorMessage(String(bytes)))
            return bytes
        } finally {
            connection.disconnect()
        }
    }

    fun request(url: String, method: String = "GET", body: String? = null, token: String? = null): String =
        String(requestBytes(url, method, body, token))

    /** Authorized request against a server path, with one refresh-and-retry on 401. */
    fun authorized(path: String, method: String = "GET", body: String? = null): String =
        String(authorizedBytes(path, method, body))

    fun authorizedBytes(path: String, method: String = "GET", body: String? = null): ByteArray {
        val used = session.tokens ?: throw HttpError(401, "sign in required")
        return try {
            requestBytes(url(path), method, body, used.accessToken)
        } catch (error: HttpError) {
            if (error.status != 401) throw error
            requestBytes(url(path), method, body, refreshIfStale(used.accessToken).accessToken)
        }
    }

    /**
     * Refreshes the session unless another caller already replaced [staleAccessToken];
     * refresh tokens rotate, so two racing refreshes would otherwise invalidate each other.
     */
    @Synchronized
    fun refreshIfStale(staleAccessToken: String): SessionTokens {
        session.tokens?.let { if (it.accessToken != staleAccessToken) return it }
        return refresh()
    }

    @Synchronized
    fun refresh(): SessionTokens {
        val refreshToken = session.tokens?.refreshToken ?: throw HttpError(401, "sign in required")
        val json = request(
            session.serverUrl + "/api/v1/sessions/refresh", "POST",
            "{\"refreshToken\":${org.json.JSONObject.quote(refreshToken)}}",
        )
        return SessionTokens.fromJson(json).also { session.tokens = it }
    }
}
