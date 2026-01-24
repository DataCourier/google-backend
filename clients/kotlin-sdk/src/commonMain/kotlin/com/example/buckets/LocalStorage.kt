package com.example.buckets

import kotlinx.serialization.json.*
import kotlin.reflect.KClass

/**
 * Local storage for records.
 * Platform implementations use SharedPreferences (Android) or UserDefaults (iOS).
 */
interface LocalStorage {
    /** Save a JSON record to local storage */
    fun save(bucketName: String, id: String, data: JsonObject)

    /** Get a record by ID */
    fun get(bucketName: String, id: String): JsonObject?

    /** Get all records for a bucket */
    fun getAll(bucketName: String): List<JsonObject>

    /** Delete a record */
    fun delete(bucketName: String, id: String)

    /** Clear all records for a bucket */
    fun clear(bucketName: String)

    /** Clear everything */
    fun clearAll()
}

/**
 * In-memory storage for testing and simple use cases.
 */
class InMemoryStorage : LocalStorage {
    private val store = mutableMapOf<String, MutableMap<String, JsonObject>>()

    override fun save(bucketName: String, id: String, data: JsonObject) {
        store.getOrPut(bucketName) { mutableMapOf() }[id] = data
    }

    override fun get(bucketName: String, id: String): JsonObject? {
        return store[bucketName]?.get(id)
    }

    override fun getAll(bucketName: String): List<JsonObject> {
        return store[bucketName]?.values?.toList() ?: emptyList()
    }

    override fun delete(bucketName: String, id: String) {
        store[bucketName]?.remove(id)
    }

    override fun clear(bucketName: String) {
        store.remove(bucketName)
    }

    override fun clearAll() {
        store.clear()
    }
}
