package com.example.buckets

/**
 * Marks a class as a Personal Bucket record.
 * Data is user-scoped and isolated.
 *
 * @param bucket The bucket name (e.g., "notes", "tasks")
 */
@Target(AnnotationTarget.CLASS)
@Retention(AnnotationRetention.RUNTIME)
annotation class PersonalBucket(val bucket: String)

/**
 * Marks a class as an Org Bucket record.
 * Data is org-scoped with role-based access.
 *
 * @param bucket The bucket name (e.g., "notes", "projects")
 */
@Target(AnnotationTarget.CLASS)
@Retention(AnnotationRetention.RUNTIME)
annotation class OrgBucket(val bucket: String)

/**
 * Marks a class as a Shared Bucket record.
 * Data can be shared between users.
 *
 * @param bucket The bucket name
 */
@Target(AnnotationTarget.CLASS)
@Retention(AnnotationRetention.RUNTIME)
annotation class SharedBucket(val bucket: String)

/**
 * Marks a field as the primary ID (auto-generated if not set).
 */
@Target(AnnotationTarget.PROPERTY)
@Retention(AnnotationRetention.RUNTIME)
annotation class Id

/**
 * Marks a field as read-only (set by server).
 */
@Target(AnnotationTarget.PROPERTY)
@Retention(AnnotationRetention.RUNTIME)
annotation class ReadOnly

/**
 * Specifies the JSON field name if different from property name.
 */
@Target(AnnotationTarget.PROPERTY)
@Retention(AnnotationRetention.RUNTIME)
annotation class Field(val name: String)
