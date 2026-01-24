package com.example.buckets

import kotlinx.datetime.Instant
import kotlinx.serialization.json.*
import kotlin.reflect.KClass
import kotlin.reflect.KMutableProperty1
import kotlin.reflect.KProperty1
import kotlin.reflect.full.findAnnotation
import kotlin.reflect.full.memberProperties

/**
 * Repository for a specific record type.
 * Provides Active Record-style operations.
 *
 * Usage:
 * ```
 * @PersonalBucket("notes")
 * class Note : PersonalRecord() {
 *     var title: String = ""
 *     var content: String = ""
 * }
 *
 * val notes = Repository(Note::class, client)
 *
 * // Create
 * val note = Note().apply { title = "Hello"; content = "World" }
 * notes.save(note)
 *
 * // Read
 * val loaded = notes.find(note.id!!)
 *
 * // Update
 * loaded.title = "Updated"
 * notes.save(loaded)
 *
 * // Delete
 * notes.delete(loaded)
 * ```
 */
class Repository<T : Record>(
    private val recordClass: KClass<T>,
    private val client: BucketClient,
    private val orgId: String? = null // Required for OrgBucket records
) {
    private val bucketInfo: BucketInfo = extractBucketInfo()

    private data class BucketInfo(
        val name: String,
        val type: BucketType
    )

    private enum class BucketType { PERSONAL, ORG, SHARED }

    private fun extractBucketInfo(): BucketInfo {
        recordClass.findAnnotation<PersonalBucket>()?.let {
            return BucketInfo(it.bucket, BucketType.PERSONAL)
        }
        recordClass.findAnnotation<OrgBucket>()?.let {
            return BucketInfo(it.bucket, BucketType.ORG)
        }
        recordClass.findAnnotation<SharedBucket>()?.let {
            return BucketInfo(it.bucket, BucketType.SHARED)
        }
        throw IllegalArgumentException("Record class must be annotated with @PersonalBucket, @OrgBucket, or @SharedBucket")
    }

    /**
     * Save a record (create or update).
     */
    suspend fun save(record: T): Result<T> {
        return if (record.isPersisted) {
            update(record)
        } else {
            create(record)
        }
    }

    /**
     * Create a new record.
     */
    suspend fun create(record: T): Result<T> {
        val json = recordToJson(record)

        val result = when (bucketInfo.type) {
            BucketType.PERSONAL -> client.createPersonal(bucketInfo.name, json)
            BucketType.ORG -> {
                val org = orgId ?: (record as? OrgRecord)?.orgId
                    ?: throw IllegalStateException("orgId required for OrgBucket records")
                client.createOrg(org, bucketInfo.name, json)
            }
            BucketType.SHARED -> client.createPersonal(bucketInfo.name, json) // TODO: shared
        }

        return result.map { data ->
            jsonToRecord(data, record)
            (record as? PersonalRecord)?.markClean()
            (record as? OrgRecord)?.markClean()
            record
        }
    }

    /**
     * Update an existing record.
     */
    suspend fun update(record: T): Result<T> {
        val id = record.id ?: throw IllegalStateException("Cannot update record without ID")
        val json = recordToJson(record)

        val result = when (bucketInfo.type) {
            BucketType.PERSONAL -> client.updatePersonal(bucketInfo.name, id, json)
            BucketType.ORG -> {
                val org = orgId ?: (record as? OrgRecord)?.orgId
                    ?: throw IllegalStateException("orgId required for OrgBucket records")
                client.updateOrg(org, bucketInfo.name, id, json)
            }
            BucketType.SHARED -> client.updatePersonal(bucketInfo.name, id, json)
        }

        return result.map { data ->
            jsonToRecord(data, record)
            (record as? PersonalRecord)?.markClean()
            (record as? OrgRecord)?.markClean()
            record
        }
    }

    /**
     * Find a record by ID.
     */
    suspend fun find(id: String): Result<T> {
        val result = when (bucketInfo.type) {
            BucketType.PERSONAL -> client.getPersonal(bucketInfo.name, id)
            BucketType.ORG -> {
                val org = orgId ?: throw IllegalStateException("orgId required for OrgBucket records")
                client.getOrg(org, bucketInfo.name, id)
            }
            BucketType.SHARED -> client.getPersonal(bucketInfo.name, id)
        }

        return result.map { data ->
            val record = createInstance()
            jsonToRecord(data, record)
            (record as? PersonalRecord)?.markClean()
            (record as? OrgRecord)?.markClean()
            record
        }
    }

    /**
     * Delete a record.
     */
    suspend fun delete(record: T): Result<Unit> {
        val id = record.id ?: throw IllegalStateException("Cannot delete record without ID")

        return when (bucketInfo.type) {
            BucketType.PERSONAL -> client.deletePersonal(bucketInfo.name, id)
            BucketType.ORG -> {
                val org = orgId ?: (record as? OrgRecord)?.orgId
                    ?: throw IllegalStateException("orgId required for OrgBucket records")
                client.deleteOrg(org, bucketInfo.name, id)
            }
            BucketType.SHARED -> client.deletePersonal(bucketInfo.name, id)
        }
    }

    /**
     * Delete a record by ID.
     */
    suspend fun delete(id: String): Result<Unit> {
        return when (bucketInfo.type) {
            BucketType.PERSONAL -> client.deletePersonal(bucketInfo.name, id)
            BucketType.ORG -> {
                val org = orgId ?: throw IllegalStateException("orgId required for OrgBucket records")
                client.deleteOrg(org, bucketInfo.name, id)
            }
            BucketType.SHARED -> client.deletePersonal(bucketInfo.name, id)
        }
    }

    /**
     * List all records (org bucket only).
     */
    suspend fun all(): Result<List<T>> {
        if (bucketInfo.type != BucketType.ORG) {
            return Result.failure(UnsupportedOperationException("all() only supported for OrgBucket"))
        }

        val org = orgId ?: throw IllegalStateException("orgId required for OrgBucket records")
        return client.listOrg(org, bucketInfo.name).map { list ->
            list.map { data ->
                val record = createInstance()
                jsonToRecord(data, record)
                (record as? OrgRecord)?.markClean()
                record
            }
        }
    }

    // =========================================================================
    // Serialization helpers
    // =========================================================================

    private fun recordToJson(record: T): JsonObject {
        val map = mutableMapOf<String, JsonElement>()

        for (prop in recordClass.memberProperties) {
            // Skip read-only fields (they're set by server)
            if (prop.findAnnotation<ReadOnly>() != null) continue
            if (prop.findAnnotation<Id>() != null && (prop.get(record) as? String).isNullOrEmpty()) continue

            val fieldName = prop.findAnnotation<Field>()?.name ?: prop.name.toSnakeCase()
            val value = prop.get(record)

            map[fieldName] = valueToJson(value)
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
        else -> JsonPrimitive(value.toString())
    }

    @Suppress("UNCHECKED_CAST")
    private fun jsonToRecord(json: JsonObject, record: T) {
        for (prop in recordClass.memberProperties) {
            if (prop !is KMutableProperty1) continue

            val fieldName = prop.findAnnotation<Field>()?.name ?: prop.name.toSnakeCase()
            val jsonValue = json[fieldName] ?: json[prop.name] ?: continue

            try {
                val value = jsonToValue(jsonValue, prop.returnType.classifier as? KClass<*>)
                (prop as KMutableProperty1<T, Any?>).set(record, value)
            } catch (e: Exception) {
                // Skip fields that can't be set
            }
        }
    }

    private fun jsonToValue(element: JsonElement, targetType: KClass<*>?): Any? = when {
        element is JsonNull -> null
        element is JsonPrimitive && element.isString -> when (targetType) {
            Instant::class -> Instant.parse(element.content)
            else -> element.content
        }
        element is JsonPrimitive -> when (targetType) {
            Int::class -> element.int
            Long::class -> element.long
            Double::class -> element.double
            Float::class -> element.float
            Boolean::class -> element.boolean
            else -> element.content
        }
        element is JsonArray -> element.map { jsonToValue(it, null) }
        element is JsonObject -> element.toMap().mapValues { jsonToValue(it.value, null) }
        else -> null
    }

    private fun createInstance(): T {
        return recordClass.constructors.first { it.parameters.isEmpty() }.call()
    }

    private fun String.toSnakeCase(): String {
        return this.replace(Regex("([a-z])([A-Z])")) {
            "${it.groupValues[1]}_${it.groupValues[2]}"
        }.lowercase()
    }
}

/**
 * Extension function to create a repository for a record type.
 */
inline fun <reified T : Record> BucketClient.repository(orgId: String? = null): Repository<T> {
    return Repository(T::class, this, orgId)
}
