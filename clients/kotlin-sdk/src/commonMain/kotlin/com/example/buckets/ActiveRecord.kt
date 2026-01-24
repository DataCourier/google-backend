package com.example.buckets

import kotlinx.datetime.Clock
import kotlinx.datetime.Instant
import kotlinx.serialization.json.*
import kotlin.reflect.KClass
import kotlin.reflect.KMutableProperty1
import kotlin.reflect.full.findAnnotation
import kotlin.reflect.full.memberProperties
import kotlin.uuid.ExperimentalUuidApi
import kotlin.uuid.Uuid

/**
 * Base class for all Active Record models.
 *
 * Usage:
 * ```kotlin
 * @PersonalBucket("notes")
 * class Note(
 *     var title: String = "",
 *     var content: String = ""
 * ) : PersonalActiveRecord()
 *
 * val note = Note(title = "Hello")
 * note.save()  // Instant local save, background sync
 * ```
 */
abstract class ActiveRecord {
    @OptIn(ExperimentalUuidApi::class)
    var id: String = Uuid.random().toString()

    var syncState: SyncState = SyncState.DIRTY
    var createdAt: Instant = Clock.System.now()
    var updatedAt: Instant = Clock.System.now()

    /** Bucket name from annotation */
    abstract val bucketName: String

    /**
     * Save record locally (instant) and queue sync to backend.
     */
    fun save() {
        updatedAt = Clock.System.now()
        if (syncState != SyncState.DELETED) {
            syncState = SyncState.DIRTY
        }

        // Save to local storage immediately
        val json = toJson()
        BucketContext.storage.save(bucketName, id, json)

        // Queue background sync
        BucketContext.queueSync(this)
    }

    /**
     * Delete record locally and queue deletion on backend.
     */
    fun delete() {
        syncState = SyncState.DELETED
        updatedAt = Clock.System.now()

        // Update local storage with deleted state
        val json = toJson()
        BucketContext.storage.save(bucketName, id, json)

        // Queue background sync (will delete on server)
        BucketContext.queueSync(this)

        // Remove from local storage after sync queued
        BucketContext.storage.delete(bucketName, id)
    }

    /**
     * Serialize record to JSON.
     */
    internal fun toJson(): JsonObject {
        val map = mutableMapOf<String, JsonElement>()

        map["id"] = JsonPrimitive(id)
        map["sync_state"] = JsonPrimitive(syncState.name)
        map["created_at"] = JsonPrimitive(createdAt.toString())
        map["updated_at"] = JsonPrimitive(updatedAt.toString())

        // Add all properties from subclass
        for (prop in this::class.memberProperties) {
            val name = prop.name
            if (name in listOf("id", "syncState", "createdAt", "updatedAt", "bucketName", "userId", "orgId")) continue

            @Suppress("UNCHECKED_CAST")
            val value = (prop as? KMutableProperty1<ActiveRecord, *>)?.get(this)
            val jsonName = prop.findAnnotation<Field>()?.name ?: name.toSnakeCase()
            map[jsonName] = valueToJson(value)
        }

        return JsonObject(map)
    }

    private fun valueToJson(value: Any?): JsonElement = when (value) {
        null -> JsonNull
        is String -> JsonPrimitive(value)
        is Number -> JsonPrimitive(value)
        is Boolean -> JsonPrimitive(value)
        is Instant -> JsonPrimitive(value.toString())
        is List<*> -> JsonArray(value.map { valueToJson(it) })
        is Map<*, *> -> JsonObject(value.entries.associate {
            it.key.toString() to valueToJson(it.value)
        })
        is Enum<*> -> JsonPrimitive(value.name)
        else -> JsonPrimitive(value.toString())
    }

    private fun String.toSnakeCase(): String {
        return this.replace(Regex("([a-z])([A-Z])")) {
            "${it.groupValues[1]}_${it.groupValues[2]}"
        }.lowercase()
    }

    companion object {
        /**
         * Find a record by ID.
         */
        inline fun <reified T : ActiveRecord> find(id: String): T? {
            val instance = T::class.java.getDeclaredConstructor().newInstance()
            val bucketName = instance.bucketName

            val json = BucketContext.storage.get(bucketName, id) ?: return null
            return fromJson(json)
        }

        /**
         * Get all records.
         */
        inline fun <reified T : ActiveRecord> all(): List<T> {
            val instance = T::class.java.getDeclaredConstructor().newInstance()
            val bucketName = instance.bucketName

            return BucketContext.storage.getAll(bucketName)
                .filter { it["sync_state"]?.jsonPrimitive?.content != SyncState.DELETED.name }
                .map { fromJson(it) }
        }

        /**
         * Filter records locally.
         */
        inline fun <reified T : ActiveRecord> where(predicate: (T) -> Boolean): List<T> {
            return all<T>().filter(predicate)
        }

        /**
         * Query records from backend with filters.
         * Results are saved to local storage.
         *
         * ```kotlin
         * // Fetch active notes from server
         * val active = Note.query(mapOf("status" to "active"))
         *
         * // Shorthand with vararg pairs
         * val active = Note.query("status" to "active", "priority" to "high")
         * ```
         */
        suspend inline fun <reified T : ActiveRecord> query(filters: Map<String, String> = emptyMap()): Result<List<T>> {
            val instance = T::class.java.getDeclaredConstructor().newInstance()
            val bucketName = instance.bucketName
            val client = BucketContext.client

            return client.listPersonal(bucketName, filters).map { jsonList ->
                jsonList.map { json ->
                    val record = fromJson<T>(json)
                    record.syncState = SyncState.SYNCED
                    // Save to local storage
                    BucketContext.storage.save(bucketName, record.id, record.toJson())
                    record
                }
            }
        }

        /**
         * Query with vararg pairs for convenience.
         *
         * ```kotlin
         * Note.query("status" to "active", "priority" to "high")
         * ```
         */
        suspend inline fun <reified T : ActiveRecord> query(vararg filters: Pair<String, String>): Result<List<T>> {
            return query(filters.toMap())
        }

        /**
         * Fetch all records from backend (no filters).
         * Shorthand for query(emptyMap()).
         */
        suspend inline fun <reified T : ActiveRecord> fetchAll(): Result<List<T>> {
            return query(emptyMap())
        }

        /**
         * Deserialize from JSON (reified version).
         */
        inline fun <reified T : ActiveRecord> fromJson(json: JsonObject): T {
            return fromJson(json, T::class)
        }

        /**
         * Deserialize from JSON (KClass version for relationships).
         */
        @Suppress("UNCHECKED_CAST")
        fun <T : ActiveRecord> fromJson(json: JsonObject, klass: KClass<T>): T {
            val instance = klass.java.getDeclaredConstructor().newInstance()

            // Set base fields
            json["id"]?.jsonPrimitive?.content?.let { instance.id = it }
            json["sync_state"]?.jsonPrimitive?.content?.let {
                try { instance.syncState = SyncState.valueOf(it) } catch (e: Exception) {}
            }
            json["created_at"]?.jsonPrimitive?.content?.let {
                try { instance.createdAt = Instant.parse(it) } catch (e: Exception) {}
            }
            json["updated_at"]?.jsonPrimitive?.content?.let {
                try { instance.updatedAt = Instant.parse(it) } catch (e: Exception) {}
            }

            // Set subclass properties
            for (prop in klass.memberProperties) {
                if (prop !is KMutableProperty1) continue
                val name = prop.name
                if (name in listOf("id", "syncState", "createdAt", "updatedAt", "bucketName")) continue

                val jsonName = prop.findAnnotation<Field>()?.name ?: name.toSnakeCase()
                val jsonValue = json[jsonName] ?: json[name] ?: continue

                try {
                    val mutableProp = prop as KMutableProperty1<T, Any?>
                    mutableProp.set(instance, jsonToValue(jsonValue))
                } catch (e: Exception) {
                    // Skip fields that can't be set
                }
            }

            return instance
        }

        fun jsonToValue(element: JsonElement): Any? = when {
            element is JsonNull -> null
            element is JsonPrimitive && element.isString -> element.content
            element is JsonPrimitive -> {
                element.booleanOrNull ?: element.intOrNull ?: element.longOrNull ?: element.doubleOrNull ?: element.content
            }
            element is JsonArray -> element.map { jsonToValue(it) }
            element is JsonObject -> element.toMap().mapValues { jsonToValue(it.value) }
            else -> null
        }

        private fun String.toSnakeCase(): String {
            return this.replace(Regex("([a-z])([A-Z])")) {
                "${it.groupValues[1]}_${it.groupValues[2]}"
            }.lowercase()
        }
    }
}

/**
 * Personal bucket record - owned by a user.
 */
abstract class PersonalActiveRecord : ActiveRecord() {
    var userId: String = BucketContext.userId

    override val bucketName: String by lazy {
        this::class.findAnnotation<PersonalBucket>()?.bucket
            ?: error("PersonalActiveRecord must have @PersonalBucket annotation")
    }

    override fun toJson(): JsonObject {
        val base = super.toJson().toMutableMap()
        base["user_id"] = JsonPrimitive(userId)
        return JsonObject(base)
    }
}

/**
 * Org bucket record - belongs to an organization.
 */
abstract class OrgActiveRecord : ActiveRecord() {
    var orgId: String? = null
    var createdBy: String = BucketContext.userId
    var visibility: String = Visibility.TEAM

    override val bucketName: String by lazy {
        this::class.findAnnotation<OrgBucket>()?.bucket
            ?: error("OrgActiveRecord must have @OrgBucket annotation")
    }

    override fun toJson(): JsonObject {
        val base = super.toJson().toMutableMap()
        base["org_id"] = JsonPrimitive(orgId ?: "")
        base["created_by"] = JsonPrimitive(createdBy)
        base["visibility"] = JsonPrimitive(visibility)
        return JsonObject(base)
    }
}
