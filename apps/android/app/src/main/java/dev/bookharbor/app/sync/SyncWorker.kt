package dev.bookharbor.app.sync

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import dev.bookharbor.app.AppGraph
import dev.bookharbor.app.library.HttpError
import dev.bookharbor.app.widget.ContinueReadingWidget
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

class ProgressSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val graph = AppGraph.get(applicationContext)
        if (graph.session.tokens == null) return@withContext Result.success() // signed out: nothing to send as this user
        try {
            val outcome = graph.syncEngine.runOnce()
            val annotationsRemain = graph.annotationSync.runOnce()
            ContinueReadingWidget.refresh(applicationContext) // positions from other devices may have moved
            // A page can stop early with events still queued (see SyncEngine); that is not
            // success, or WorkManager would drop the job with nothing left to retry it.
            if (outcome.pendingRemains || annotationsRemain) Result.retry() else Result.success()
        } catch (error: HttpError) {
            if (error.status == 401) {
                // The refresh token itself is dead, not just the access token: retrying will
                // never succeed. Drop the session so the next foreground shows sign-in, and stop
                // burning retries; the outbox is untouched and resumes once signed back in.
                graph.session.tokens = null
                Result.failure()
            } else {
                Result.retry()
            }
        } catch (_: Exception) {
            // Network down or server error: keep every queued event and back off.
            Result.retry()
        }
    }
}

object SyncScheduler {
    private const val NAME = "progress-sync"
    private const val PERIODIC_NAME = "progress-sync-periodic"

    /** One unique job; a save while one is queued just keeps the existing request. */
    fun schedule(context: Context) {
        val request = OneTimeWorkRequestBuilder<ProgressSyncWorker>()
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(NAME, ExistingWorkPolicy.KEEP, request)
    }

    /**
     * A self-healing daily sweep: if the event-triggered job above was ever dropped while the
     * outbox still had events (e.g. connectivity returned with the app closed), this catches it
     * without depending on another save or app launch. Call once at process start.
     */
    fun schedulePeriodic(context: Context) {
        val request = PeriodicWorkRequestBuilder<ProgressSyncWorker>(1, TimeUnit.DAYS)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
            .build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(PERIODIC_NAME, ExistingPeriodicWorkPolicy.KEEP, request)
    }
}
