package dev.bookharbor.app

import android.content.Context
import dev.bookharbor.app.library.ApiClient
import dev.bookharbor.app.library.DownloadStore
import dev.bookharbor.app.library.EditionDownloader
import dev.bookharbor.app.library.LibraryClient
import dev.bookharbor.app.library.SessionStore
import dev.bookharbor.app.sync.HttpSyncApi
import dev.bookharbor.app.sync.ProgressRecorder
import dev.bookharbor.app.sync.SqliteProgressStore
import dev.bookharbor.app.sync.SyncEngine
import dev.bookharbor.app.sync.SyncScheduler
import java.io.File
import java.util.UUID

/** Application-wide singletons, shared by the UI and the background sync worker. */
class AppGraph private constructor(context: Context) {
    private val app = context.applicationContext
    val context: Context get() = app
    val prefs = app.getSharedPreferences("bookharbor", Context.MODE_PRIVATE)
    val cacheDir: File get() = app.cacheDir

    val session = SessionStore(prefs)
    val api = ApiClient(session)
    val library = LibraryClient(api)
    val downloads = DownloadStore(prefs, File(app.filesDir, "downloads"))
    val downloader = EditionDownloader(api, downloads)
    val progress = SqliteProgressStore(app)
    val syncEngine = SyncEngine(progress, HttpSyncApi(api))

    /** Stable per install; identifies which device wrote a reading event. */
    val deviceId: String = prefs.getString("device_id", null) ?: ("device_" + UUID.randomUUID().toString().replace("-", "")).also {
        prefs.edit().putString("device_id", it).apply()
    }

    val recorder = ProgressRecorder(progress, deviceId, onRecorded = { SyncScheduler.schedule(app) })

    init {
        SyncScheduler.schedulePeriodic(app)
    }

    companion object {
        @Volatile private var instance: AppGraph? = null
        fun get(context: Context): AppGraph = instance ?: synchronized(this) { instance ?: AppGraph(context).also { instance = it } }
    }
}
