package dev.bookharbor.app.sync

import dev.bookharbor.app.library.ApiClient
import dev.bookharbor.app.library.HttpError

class HttpSyncApi(private val api: ApiClient) : SyncApi {
    override fun sync(cursor: Long, changes: List<ProgressEvent>): SyncResponse = try {
        ProgressCodec.decodeResponse(api.authorized("/api/v1/progress/sync", "POST", ProgressCodec.encodeRequest(cursor, changes)))
    } catch (error: HttpError) {
        // A 4xx the client cannot fix by retrying (bad locator, clock skew, unknown edition).
        // 401 (session), 408 and 429 are transient and must keep the event queued.
        if (error.status in 400..499 && error.status !in setOf(401, 408, 429)) throw PermanentSyncError(error.status, error.message ?: "rejected")
        throw error
    }
}
