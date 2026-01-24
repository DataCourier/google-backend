package com.example.buckets

/**
 * Tracks sync state of a record.
 */
enum class SyncState {
    /** New or modified locally, needs push to server */
    DIRTY,

    /** Matches server state */
    SYNCED,

    /** Marked for deletion, needs delete on server */
    DELETED
}
