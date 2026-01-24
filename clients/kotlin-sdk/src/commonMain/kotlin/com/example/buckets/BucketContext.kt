package com.example.buckets

import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * Global context for bucket operations.
 * Configure once at app startup.
 */
object BucketContext {
    private var _storage: LocalStorage = InMemoryStorage()
    private var _client: BucketClient? = null
    private var _userId: String = "local:anonymous"
    private var _syncScope: CoroutineScope? = null

    private val _syncStatus = MutableStateFlow<SyncStatus>(SyncStatus.Idle)
    val syncStatus: StateFlow<SyncStatus> = _syncStatus

    val storage: LocalStorage get() = _storage
    val client: BucketClient get() = _client ?: error("BucketContext not configured. Call configure() first.")
    val userId: String get() = _userId

    /**
     * Configure the bucket context.
     *
     * ```kotlin
     * BucketContext.configure {
     *     baseUrl = "http://localhost:8080"
     *     userId = "local:test-user"
     *     storage = InMemoryStorage()  // or platform-specific
     * }
     * ```
     */
    fun configure(block: Configuration.() -> Unit) {
        val config = Configuration().apply(block)

        _storage = config.storage ?: InMemoryStorage()
        _userId = config.userId ?: "local:anonymous"

        if (config.baseUrl != null) {
            _client = BucketClient(config.baseUrl!!).apply {
                setAuthToken(_userId)
            }
        }

        _syncScope = CoroutineScope(Dispatchers.Default + SupervisorJob())
    }

    /**
     * Queue a sync operation (debounced).
     */
    internal fun queueSync(record: ActiveRecord) {
        _syncScope?.launch {
            delay(100) // Small debounce
            syncRecord(record)
        }
    }

    /**
     * Sync a single record to the backend.
     */
    private suspend fun syncRecord(record: ActiveRecord) {
        val client = _client ?: return // No client = offline only

        _syncStatus.value = SyncStatus.Syncing

        try {
            val result = when (record.syncState) {
                SyncState.DIRTY -> pushRecord(record)
                SyncState.DELETED -> deleteRecord(record)
                SyncState.SYNCED -> Result.success(Unit) // Nothing to do
            }

            result.onSuccess {
                record.syncState = SyncState.SYNCED
                // Re-save with updated sync state
                val json = record.toJson()
                storage.save(record.bucketName, record.id, json)
                _syncStatus.value = SyncStatus.Synced
            }.onFailure { error ->
                _syncStatus.value = SyncStatus.Error(error.message ?: "Sync failed")
            }
        } catch (e: Exception) {
            _syncStatus.value = SyncStatus.Error(e.message ?: "Sync failed")
        }
    }

    private suspend fun pushRecord(record: ActiveRecord): Result<Unit> {
        val json = record.toJson()

        return when (record) {
            is PersonalActiveRecord -> {
                client.createPersonal(record.bucketName, json).map { }
            }
            is OrgActiveRecord -> {
                val orgId = record.orgId ?: error("OrgActiveRecord requires orgId")
                client.createOrg(orgId, record.bucketName, json).map { }
            }
            else -> Result.failure(IllegalStateException("Unknown record type"))
        }
    }

    private suspend fun deleteRecord(record: ActiveRecord): Result<Unit> {
        return when (record) {
            is PersonalActiveRecord -> {
                client.deletePersonal(record.bucketName, record.id)
            }
            is OrgActiveRecord -> {
                val orgId = record.orgId ?: error("OrgActiveRecord requires orgId")
                client.deleteOrg(orgId, record.bucketName, record.id)
            }
            else -> Result.failure(IllegalStateException("Unknown record type"))
        }
    }

    /**
     * Sync all dirty records to backend using batch API.
     * Groups records by bucket and sends in batches of up to 50 records.
     */
    suspend fun syncAll(): Result<SyncReport> {
        val client = _client ?: return Result.failure(IllegalStateException("No client configured"))

        _syncStatus.value = SyncStatus.Syncing

        val report = SyncReport()

        try {
            // Get all buckets that have data
            val buckets = storage.getAllBuckets()

            for (bucketName in buckets) {
                val allRecords = storage.getAll(bucketName)

                // Filter dirty/deleted records
                val dirtyRecords = allRecords.filter { json ->
                    val state = json["sync_state"]?.jsonPrimitive?.content
                    state == SyncState.DIRTY.name || state == SyncState.DELETED.name
                }

                if (dirtyRecords.isEmpty()) continue

                // Split into batches of 50
                val batches = dirtyRecords.chunked(MAX_BATCH_SIZE)

                for (batch in batches) {
                    // Separate deletes from creates/updates
                    val toDelete = batch.filter {
                        it["sync_state"]?.jsonPrimitive?.content == SyncState.DELETED.name
                    }
                    val toUpsert = batch.filter {
                        it["sync_state"]?.jsonPrimitive?.content == SyncState.DIRTY.name
                    }

                    // Handle deletes individually (batch delete not supported yet)
                    for (deleteJson in toDelete) {
                        val id = deleteJson["id"]?.jsonPrimitive?.content ?: continue
                        client.deletePersonal(bucketName, id)
                            .onSuccess {
                                storage.delete(bucketName, id)
                                report.deleted++
                            }
                            .onFailure {
                                report.errors++
                            }
                    }

                    // Batch upsert
                    if (toUpsert.isNotEmpty()) {
                        client.batchPersonal(bucketName, toUpsert)
                            .onSuccess { results ->
                                for (result in results) {
                                    when (result.status) {
                                        "created" -> {
                                            report.created++
                                            markSynced(bucketName, result.id)
                                        }
                                        "updated" -> {
                                            report.updated++
                                            markSynced(bucketName, result.id)
                                        }
                                        "error" -> report.errors++
                                    }
                                }
                            }
                            .onFailure {
                                report.errors += toUpsert.size
                            }
                    }
                }
            }

            _syncStatus.value = if (report.errors == 0) SyncStatus.Synced
                               else SyncStatus.Error("${report.errors} records failed to sync")

            return Result.success(report)
        } catch (e: Exception) {
            _syncStatus.value = SyncStatus.Error(e.message ?: "Sync failed")
            return Result.failure(e)
        }
    }

    private fun markSynced(bucketName: String, id: String) {
        val json = storage.get(bucketName, id) ?: return
        val mutableMap = json.toMutableMap()
        mutableMap["sync_state"] = kotlinx.serialization.json.JsonPrimitive(SyncState.SYNCED.name)
        storage.save(bucketName, id, JsonObject(mutableMap))
    }

    companion object {
        const val MAX_BATCH_SIZE = 50
    }

    /**
     * Pull all records from backend for a bucket.
     */
    suspend fun <T : ActiveRecord> pullAll(
        bucketName: String,
        parser: (JsonObject) -> T
    ): Result<List<T>> {
        val client = _client ?: return Result.failure(IllegalStateException("No client configured"))

        _syncStatus.value = SyncStatus.Syncing

        return try {
            val result = client.listPersonal(bucketName)
            result.map { jsonList ->
                jsonList.map { json ->
                    val record = parser(json)
                    record.syncState = SyncState.SYNCED
                    // Save to local storage
                    storage.save(bucketName, record.id, record.toJson())
                    record
                }
            }.also {
                _syncStatus.value = SyncStatus.Synced
            }
        } catch (e: Exception) {
            _syncStatus.value = SyncStatus.Error(e.message ?: "Pull failed")
            Result.failure(e)
        }
    }

    /**
     * Clear local storage and pull fresh from backend.
     * Use this to verify data integrity or restore from server.
     */
    suspend fun <T : ActiveRecord> refreshFromBackend(
        bucketName: String,
        parser: (JsonObject) -> T
    ): Result<List<T>> {
        storage.clear(bucketName)
        return pullAll(bucketName, parser)
    }

    /**
     * Wipe local and verify against backend.
     * Returns true if local and remote match after refresh.
     */
    suspend fun <T : ActiveRecord> verifyWithBackend(
        bucketName: String,
        parser: (JsonObject) -> T,
        comparator: (T, T) -> Boolean = { a, b -> a.id == b.id }
    ): Result<Boolean> {
        val localBefore = storage.getAll(bucketName)
        val localIds = localBefore.mapNotNull { it["id"]?.jsonPrimitive?.content }.toSet()

        return refreshFromBackend(bucketName, parser).map { remoteRecords ->
            val remoteIds = remoteRecords.map { it.id }.toSet()
            localIds == remoteIds
        }
    }

    class Configuration {
        var baseUrl: String? = null
        var userId: String? = null
        var storage: LocalStorage? = null
    }
}

sealed class SyncStatus {
    object Idle : SyncStatus()
    object Syncing : SyncStatus()
    object Synced : SyncStatus()
    data class Error(val message: String) : SyncStatus()
}

/**
 * Report from a syncAll() operation.
 */
data class SyncReport(
    var created: Int = 0,
    var updated: Int = 0,
    var deleted: Int = 0,
    var errors: Int = 0
) {
    val total: Int get() = created + updated + deleted
    val success: Boolean get() = errors == 0
}
