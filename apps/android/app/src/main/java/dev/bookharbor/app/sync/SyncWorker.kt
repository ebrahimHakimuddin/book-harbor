package dev.bookharbor.app.sync

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import dev.bookharbor.app.AppGraph
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

class ProgressSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val graph = AppGraph.get(applicationContext)
        if (graph.session.tokens == null) return@withContext Result.success() // signed out: nothing to send as this user
        try {
            graph.syncEngine.runOnce()
            Result.success()
        } catch (_: Exception) {
            // Network down, server error, or expired session: keep every queued event and back off.
            Result.retry()
        }
    }
}

object SyncScheduler {
    private const val NAME = "progress-sync"

    /** One unique job; a save while one is queued just keeps the existing request. */
    fun schedule(context: Context) {
        val request = OneTimeWorkRequestBuilder<ProgressSyncWorker>()
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(NAME, ExistingWorkPolicy.KEEP, request)
    }
}
