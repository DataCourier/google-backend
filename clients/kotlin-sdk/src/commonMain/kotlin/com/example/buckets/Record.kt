package com.example.buckets

import kotlinx.datetime.Instant

/**
 * Base interface for all bucket records.
 */
interface Record {
    var id: String?
    val createdAt: Instant?
    val updatedAt: Instant?

    /** Returns true if this record has been saved (has an ID). */
    val isPersisted: Boolean get() = id != null

    /** Returns true if this record has unsaved changes. */
    val isDirty: Boolean
}

/**
 * Base class for Personal Bucket records.
 * Automatically scoped to the current user.
 */
abstract class PersonalRecord : Record {
    @Id
    override var id: String? = null

    @ReadOnly
    override var createdAt: Instant? = null

    @ReadOnly
    override var updatedAt: Instant? = null

    @ReadOnly
    var userId: String? = null

    private var _originalHash: Int = 0

    override val isDirty: Boolean
        get() = _originalHash != this.hashCode()

    internal fun markClean() {
        _originalHash = this.hashCode()
    }
}

/**
 * Base class for Org Bucket records.
 * Scoped to an organization with role-based access.
 */
abstract class OrgRecord : Record {
    @Id
    override var id: String? = null

    @ReadOnly
    override var createdAt: Instant? = null

    @ReadOnly
    override var updatedAt: Instant? = null

    @ReadOnly
    var orgId: String? = null

    @ReadOnly
    var createdBy: String? = null

    /** Visibility: private, invite-only, team, org-wide */
    var visibility: String = "team"

    private var _originalHash: Int = 0

    override val isDirty: Boolean
        get() = _originalHash != this.hashCode()

    internal fun markClean() {
        _originalHash = this.hashCode()
    }
}

/**
 * Visibility constants for org records.
 */
object Visibility {
    const val PRIVATE = "private"
    const val INVITE_ONLY = "invite-only"
    const val TEAM = "team"
    const val ORG_WIDE = "org-wide"
}
