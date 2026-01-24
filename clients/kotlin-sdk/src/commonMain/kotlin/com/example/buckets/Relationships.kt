package com.example.buckets

import kotlin.reflect.KClass
import kotlin.reflect.KProperty

// =============================================================================
// Relationship Annotations
// =============================================================================

/**
 * Marks a foreign key field that references a parent record.
 * The field will be auto-filled when created via parent.children.create()
 *
 * ```kotlin
 * @PersonalBucket("comments")
 * class Comment : PersonalActiveRecord() {
 *     var text: String = ""
 *
 *     @BelongsTo(Post::class)
 *     var postId: String = ""
 * }
 * ```
 */
@Target(AnnotationTarget.PROPERTY)
@Retention(AnnotationRetention.RUNTIME)
annotation class BelongsTo(
    val parent: KClass<out ActiveRecord>,
    val foreignKey: String = ""  // If empty, inferred as parentClassName + "Id"
)

/**
 * Declares a one-to-many relationship.
 *
 * ```kotlin
 * @PersonalBucket("posts")
 * class Post : PersonalActiveRecord() {
 *     val comments by hasMany<Comment>()
 * }
 * ```
 */
@Target(AnnotationTarget.PROPERTY)
@Retention(AnnotationRetention.RUNTIME)
annotation class HasManyAnnotation(
    val foreignKey: String = ""  // If empty, inferred as thisClassName + "Id"
)

/**
 * Declares a one-to-one relationship.
 *
 * ```kotlin
 * @PersonalBucket("users")
 * class User : PersonalActiveRecord() {
 *     val profile by hasOne<Profile>()
 * }
 * ```
 */
@Target(AnnotationTarget.PROPERTY)
@Retention(AnnotationRetention.RUNTIME)
annotation class HasOneAnnotation(
    val foreignKey: String = ""
)

// =============================================================================
// HasMany Delegate
// =============================================================================

/**
 * Delegate for has-many relationships.
 * Provides create(), all(), find(), where() scoped to the parent.
 */
class HasMany<T : ActiveRecord>(
    private val parent: ActiveRecord,
    private val childClass: KClass<T>,
    private val foreignKey: String
) {
    /**
     * Create a new child record with foreign key auto-filled.
     */
    inline fun <reified T : ActiveRecord> create(block: T.() -> Unit): T {
        val child = T::class.java.getDeclaredConstructor().newInstance()
        child.block()
        setForeignKey(child)
        child.save()
        return child
    }

    /**
     * Create without lambda.
     */
    fun build(): T {
        val child = childClass.java.getDeclaredConstructor().newInstance()
        setForeignKey(child)
        return child
    }

    /**
     * Get all children belonging to this parent.
     */
    fun all(): List<T> {
        val allRecords = getAllRecords()
        return allRecords.filter { record ->
            getForeignKeyValue(record) == parent.id
        }
    }

    /**
     * Find a child by ID (only if it belongs to this parent).
     */
    fun find(id: String): T? {
        val record = findRecord(id) ?: return null
        return if (getForeignKeyValue(record) == parent.id) record else null
    }

    /**
     * Filter children with a predicate.
     */
    fun where(predicate: (T) -> Boolean): List<T> {
        return all().filter(predicate)
    }

    /**
     * Count children.
     */
    fun count(): Int = all().size

    /**
     * Check if any children exist.
     */
    fun isEmpty(): Boolean = all().isEmpty()
    fun isNotEmpty(): Boolean = all().isNotEmpty()

    // -------------------------------------------------------------------------
    // Internal helpers
    // -------------------------------------------------------------------------

    private fun setForeignKey(child: T) {
        val prop = child::class.java.getDeclaredField(foreignKey)
        prop.isAccessible = true
        prop.set(child, parent.id)
    }

    private fun getForeignKeyValue(child: T): String? {
        return try {
            val prop = child::class.java.getDeclaredField(foreignKey)
            prop.isAccessible = true
            prop.get(child) as? String
        } catch (e: Exception) {
            null
        }
    }

    @Suppress("UNCHECKED_CAST")
    private fun getAllRecords(): List<T> {
        val instance = childClass.java.getDeclaredConstructor().newInstance()
        val bucketName = instance.bucketName
        return BucketContext.storage.getAll(bucketName)
            .filter { it["sync_state"]?.toString()?.trim('"') != SyncState.DELETED.name }
            .map { json -> ActiveRecord.fromJson<T>(json, childClass) }
    }

    @Suppress("UNCHECKED_CAST")
    private fun findRecord(id: String): T? {
        val instance = childClass.java.getDeclaredConstructor().newInstance()
        val bucketName = instance.bucketName
        val json = BucketContext.storage.get(bucketName, id) ?: return null
        return ActiveRecord.fromJson<T>(json, childClass)
    }
}

// =============================================================================
// HasOne Delegate
// =============================================================================

/**
 * Delegate for has-one relationships.
 */
class HasOne<T : ActiveRecord>(
    private val parent: ActiveRecord,
    private val childClass: KClass<T>,
    private val foreignKey: String
) {
    /**
     * Get the related record (or null).
     */
    fun get(): T? {
        val allRecords = getAllRecords()
        return allRecords.find { record ->
            getForeignKeyValue(record) == parent.id
        }
    }

    /**
     * Create or replace the related record.
     */
    fun create(block: T.() -> Unit): T {
        // Delete existing if any
        get()?.delete()

        val child = childClass.java.getDeclaredConstructor().newInstance()
        child.block()
        setForeignKey(child)
        child.save()
        return child
    }

    /**
     * Build without saving.
     */
    fun build(): T {
        val child = childClass.java.getDeclaredConstructor().newInstance()
        setForeignKey(child)
        return child
    }

    // -------------------------------------------------------------------------
    // Internal helpers
    // -------------------------------------------------------------------------

    private fun setForeignKey(child: T) {
        val prop = child::class.java.getDeclaredField(foreignKey)
        prop.isAccessible = true
        prop.set(child, parent.id)
    }

    private fun getForeignKeyValue(child: T): String? {
        return try {
            val prop = child::class.java.getDeclaredField(foreignKey)
            prop.isAccessible = true
            prop.get(child) as? String
        } catch (e: Exception) {
            null
        }
    }

    @Suppress("UNCHECKED_CAST")
    private fun getAllRecords(): List<T> {
        val instance = childClass.java.getDeclaredConstructor().newInstance()
        val bucketName = instance.bucketName
        return BucketContext.storage.getAll(bucketName)
            .filter { it["sync_state"]?.toString()?.trim('"') != SyncState.DELETED.name }
            .map { json -> ActiveRecord.fromJson<T>(json, childClass) }
    }
}

// =============================================================================
// Property Delegates (for clean syntax)
// =============================================================================

/**
 * Create a HasMany delegate.
 *
 * ```kotlin
 * class Post : PersonalActiveRecord() {
 *     val comments by hasMany<Comment>()
 *     val tags by hasMany<Tag>("postId")  // explicit foreign key
 * }
 * ```
 */
inline fun <reified T : ActiveRecord> ActiveRecord.hasMany(
    foreignKey: String? = null
): HasManyDelegate<T> {
    return HasManyDelegate(T::class, foreignKey)
}

class HasManyDelegate<T : ActiveRecord>(
    private val childClass: KClass<T>,
    private val explicitForeignKey: String?
) {
    operator fun getValue(thisRef: ActiveRecord, property: KProperty<*>): HasMany<T> {
        val fk = explicitForeignKey ?: inferForeignKey(thisRef::class)
        return HasMany(thisRef, childClass, fk)
    }

    private fun inferForeignKey(parentClass: KClass<out ActiveRecord>): String {
        val name = parentClass.simpleName ?: "Parent"
        return name.replaceFirstChar { it.lowercase() } + "Id"
    }
}

/**
 * Create a HasOne delegate.
 *
 * ```kotlin
 * class User : PersonalActiveRecord() {
 *     val profile by hasOne<Profile>()
 * }
 * ```
 */
inline fun <reified T : ActiveRecord> ActiveRecord.hasOne(
    foreignKey: String? = null
): HasOneDelegate<T> {
    return HasOneDelegate(T::class, foreignKey)
}

class HasOneDelegate<T : ActiveRecord>(
    private val childClass: KClass<T>,
    private val explicitForeignKey: String?
) {
    operator fun getValue(thisRef: ActiveRecord, property: KProperty<*>): HasOne<T> {
        val fk = explicitForeignKey ?: inferForeignKey(thisRef::class)
        return HasOne(thisRef, childClass, fk)
    }

    private fun inferForeignKey(parentClass: KClass<out ActiveRecord>): String {
        val name = parentClass.simpleName ?: "Parent"
        return name.replaceFirstChar { it.lowercase() } + "Id"
    }
}

// =============================================================================
// BelongsTo helper
// =============================================================================

/**
 * Get the parent record for a BelongsTo relationship.
 *
 * ```kotlin
 * class Comment : PersonalActiveRecord() {
 *     @BelongsTo(Post::class)
 *     var postId: String = ""
 *
 *     val post by belongsTo<Post>(::postId)
 * }
 * ```
 */
inline fun <reified T : ActiveRecord> ActiveRecord.belongsTo(
    crossinline foreignKeyGetter: () -> String
): BelongsToDelegate<T> {
    return BelongsToDelegate(T::class, foreignKeyGetter)
}

class BelongsToDelegate<T : ActiveRecord>(
    private val parentClass: KClass<T>,
    private val foreignKeyGetter: () -> String
) {
    private var cached: T? = null
    private var cachedId: String? = null

    operator fun getValue(thisRef: ActiveRecord, property: KProperty<*>): T? {
        val fkValue = foreignKeyGetter()
        if (fkValue.isEmpty()) return null

        // Return cached if FK hasn't changed
        if (cachedId == fkValue && cached != null) {
            return cached
        }

        // Look up parent
        val instance = parentClass.java.getDeclaredConstructor().newInstance()
        val bucketName = instance.bucketName
        val json = BucketContext.storage.get(bucketName, fkValue) ?: return null

        cached = ActiveRecord.fromJson(json, parentClass)
        cachedId = fkValue
        return cached
    }
}
