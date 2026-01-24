package com.example.buckets

import kotlinx.coroutines.delay
import kotlinx.serialization.json.JsonObject

/**
 * Example: Offline-first flow with sync and verification.
 *
 * The key insight:
 * 1. save() is INSTANT (local only)
 * 2. Sync happens in background
 * 3. You can wipe local and pull from backend to verify
 */
suspend fun offlineFirstExample() {
    // =========================================================================
    // Setup
    // =========================================================================

    BucketContext.configure {
        baseUrl = "http://localhost:8080"
        userId = "local:test-user"
        storage = InMemoryStorage()
    }

    // =========================================================================
    // Phase 1: Local-only changes (instant)
    // =========================================================================

    println("Creating notes locally...")

    val note1 = Note(title = "Shopping", content = "Milk, eggs")
    note1.save()  // INSTANT - saved to local storage
    println("  Created: ${note1.id} (sync: ${note1.syncState})")

    val note2 = Note(title = "Work", content = "Finish report")
    note2.save()
    println("  Created: ${note2.id} (sync: ${note2.syncState})")

    val note3 = Note(title = "Ideas", content = "App improvements")
    note3.save()
    println("  Created: ${note3.id} (sync: ${note3.syncState})")

    // All notes are DIRTY (not yet synced)
    println("\nLocal notes: ${Note.all().size}")
    Note.all().forEach { note ->
        println("  - ${note.title} [${note.syncState}]")
    }

    // =========================================================================
    // Phase 2: Background sync
    // =========================================================================

    println("\nWaiting for background sync...")
    delay(500)  // Let the background sync complete

    // Check sync status
    println("Sync status: ${BucketContext.syncStatus.value}")

    // Notes should now be SYNCED
    Note.all().forEach { note ->
        println("  - ${note.title} [${note.syncState}]")
    }

    // =========================================================================
    // Phase 3: Verify by wiping local and pulling from backend
    // =========================================================================

    println("\nWiping local storage...")
    BucketContext.storage.clear("notes")
    println("Local notes after wipe: ${Note.all().size}")

    println("\nPulling from backend...")
    BucketContext.refreshFromBackend("notes") { json ->
        Note().apply {
            id = json["id"]?.toString()?.trim('"') ?: id
            // Parse other fields...
        }
    }.onSuccess { notes ->
        println("Retrieved ${notes.size} notes from backend")
        notes.forEach { note ->
            println("  - ${note.id} [${note.syncState}]")
        }
    }.onFailure { error ->
        println("Failed to pull: ${error.message}")
    }
}

/**
 * Example: The simple API users will actually use.
 */
suspend fun simpleUsageExample() {
    // Configure once at app startup
    BucketContext.configure {
        baseUrl = "http://localhost:8080"
        userId = "local:alice"
    }

    // Create - instant
    val note = Note(title = "Hello", content = "World")
    note.save()

    // Update - instant
    note.title = "Updated"
    note.save()

    // Read
    val loaded = Note.find(note.id)
    println("Found: ${loaded?.title}")

    // Query
    val all = Note.all()
    val pinned = Note.where { it.isPinned }

    // Delete - instant
    note.delete()

    // Sync happens automatically in background
    // Check status if you care:
    println("Status: ${BucketContext.syncStatus.value}")
}
