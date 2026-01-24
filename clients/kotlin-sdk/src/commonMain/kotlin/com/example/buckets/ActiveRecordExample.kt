package com.example.buckets

/**
 * Example models using Active Record pattern.
 */

// =============================================================================
// Personal Bucket Examples
// =============================================================================

@PersonalBucket("notes")
class Note(
    var title: String = "",
    var content: String = "",
    var isPinned: Boolean = false,
    var tags: List<String> = emptyList()
) : PersonalActiveRecord() {

    companion object {
        fun find(id: String): Note? = ActiveRecord.find(id)
        fun all(): List<Note> = ActiveRecord.all()
        fun where(predicate: (Note) -> Boolean): List<Note> = ActiveRecord.where(predicate)
    }
}

@PersonalBucket("tasks")
class Task(
    var title: String = "",
    var completed: Boolean = false,
    var priority: Int = 0
) : PersonalActiveRecord() {

    companion object {
        fun find(id: String): Task? = ActiveRecord.find(id)
        fun all(): List<Task> = ActiveRecord.all()
        fun where(predicate: (Task) -> Boolean): List<Task> = ActiveRecord.where(predicate)
    }
}

// =============================================================================
// Org Bucket Examples
// =============================================================================

@OrgBucket("projects")
class Project(
    var name: String = "",
    var description: String = "",
    var status: String = "active"
) : OrgActiveRecord() {

    companion object {
        fun find(id: String): Project? = ActiveRecord.find(id)
        fun all(): List<Project> = ActiveRecord.all()
    }
}

// =============================================================================
// Usage Example
// =============================================================================

fun activeRecordExample() {
    // Configure once at app startup
    BucketContext.configure {
        baseUrl = "http://localhost:8080"
        userId = "local:test-user"
        storage = InMemoryStorage()
    }

    // ---------------------------------------------------------------------
    // Create - instant local save
    // ---------------------------------------------------------------------
    val note = Note(
        title = "Shopping List",
        content = "Milk, eggs, bread",
        tags = listOf("shopping", "groceries")
    )
    note.save()  // Instant! Syncs in background

    println("Created note: ${note.id}")
    println("Sync state: ${note.syncState}")  // DIRTY until synced

    // ---------------------------------------------------------------------
    // Read
    // ---------------------------------------------------------------------
    val loaded = Note.find(note.id)
    println("Loaded: ${loaded?.title}")

    // ---------------------------------------------------------------------
    // List all
    // ---------------------------------------------------------------------
    val allNotes = Note.all()
    println("Total notes: ${allNotes.size}")

    // ---------------------------------------------------------------------
    // Query/Filter
    // ---------------------------------------------------------------------
    val pinnedNotes = Note.where { it.isPinned }
    println("Pinned notes: ${pinnedNotes.size}")

    // ---------------------------------------------------------------------
    // Update
    // ---------------------------------------------------------------------
    note.title = "Updated Shopping List"
    note.isPinned = true
    note.save()  // Instant! Syncs in background

    // ---------------------------------------------------------------------
    // Delete
    // ---------------------------------------------------------------------
    note.delete()  // Instant! Deletes on server in background

    // ---------------------------------------------------------------------
    // Check sync status
    // ---------------------------------------------------------------------
    // BucketContext.syncStatus.collect { status ->
    //     when (status) {
    //         is SyncStatus.Synced -> println("All synced!")
    //         is SyncStatus.Syncing -> println("Syncing...")
    //         is SyncStatus.Error -> println("Error: ${status.message}")
    //         is SyncStatus.Idle -> println("Idle")
    //     }
    // }
}
