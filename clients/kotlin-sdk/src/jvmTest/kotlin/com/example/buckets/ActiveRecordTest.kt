package com.example.buckets

import kotlin.test.*

/**
 * Tests for ActiveRecord base functionality.
 */
class ActiveRecordTest {

    @BeforeTest
    fun setup() {
        BucketContext.configure {
            storage = InMemoryStorage()
            userId = "test-user"
        }
    }

    @AfterTest
    fun cleanup() {
        BucketContext.storage.clearAll()
    }

    // =========================================================================
    // Save Tests
    // =========================================================================

    @Test
    fun `save persists record to local storage`() {
        val note = SimpleNote(title = "Hello", content = "World")
        note.save()

        val loaded = SimpleNote.find(note.id)

        assertNotNull(loaded)
        assertEquals("Hello", loaded.title)
        assertEquals("World", loaded.content)
    }

    @Test
    fun `save generates UUID for new records`() {
        val note = SimpleNote(title = "Test")

        assertNotNull(note.id)
        assertTrue(note.id.isNotEmpty())
        // UUID format: 8-4-4-4-12 hex chars
        assertTrue(note.id.matches(Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")))
    }

    @Test
    fun `save marks record as dirty`() {
        val note = SimpleNote(title = "Test")

        assertEquals(SyncState.DIRTY, note.syncState)

        note.save()

        // Still dirty until synced
        assertEquals(SyncState.DIRTY, note.syncState)
    }

    @Test
    fun `save updates existing record`() {
        val note = SimpleNote(title = "Original")
        note.save()
        val originalId = note.id

        note.title = "Updated"
        note.save()

        val loaded = SimpleNote.find(originalId)
        assertEquals("Updated", loaded?.title)
        assertEquals(originalId, loaded?.id)
    }

    @Test
    fun `save updates updatedAt timestamp`() {
        val note = SimpleNote(title = "Test")
        val originalUpdatedAt = note.updatedAt

        Thread.sleep(10) // Ensure time passes
        note.save()

        assertTrue(note.updatedAt >= originalUpdatedAt)
    }

    // =========================================================================
    // Find Tests
    // =========================================================================

    @Test
    fun `find returns null for non-existent id`() {
        val result = SimpleNote.find("non-existent-id")
        assertNull(result)
    }

    @Test
    fun `find returns record by id`() {
        val note = SimpleNote(title = "Find Me", content = "Here I am")
        note.save()

        val found = SimpleNote.find(note.id)

        assertNotNull(found)
        assertEquals(note.id, found.id)
        assertEquals("Find Me", found.title)
        assertEquals("Here I am", found.content)
    }

    // =========================================================================
    // All Tests
    // =========================================================================

    @Test
    fun `all returns empty list when no records exist`() {
        val notes = SimpleNote.all()
        assertTrue(notes.isEmpty())
    }

    @Test
    fun `all returns all records`() {
        SimpleNote(title = "Note 1").save()
        SimpleNote(title = "Note 2").save()
        SimpleNote(title = "Note 3").save()

        val notes = SimpleNote.all()

        assertEquals(3, notes.size)
        assertTrue(notes.any { it.title == "Note 1" })
        assertTrue(notes.any { it.title == "Note 2" })
        assertTrue(notes.any { it.title == "Note 3" })
    }

    @Test
    fun `all excludes deleted records`() {
        val note1 = SimpleNote(title = "Keep")
        note1.save()

        val note2 = SimpleNote(title = "Delete")
        note2.save()
        note2.delete()

        val notes = SimpleNote.all()

        assertEquals(1, notes.size)
        assertEquals("Keep", notes.first().title)
    }

    // =========================================================================
    // Where Tests
    // =========================================================================

    @Test
    fun `where filters records by predicate`() {
        SimpleNote(title = "Important", content = "Do this").save()
        SimpleNote(title = "Also Important", content = "And this").save()
        SimpleNote(title = "Not important", content = "Skip").save()

        val important = SimpleNote.where { it.title.contains("Important") }

        assertEquals(2, important.size)
        assertTrue(important.all { it.title.contains("Important") })
    }

    @Test
    fun `where returns empty list when no matches`() {
        SimpleNote(title = "Hello").save()

        val results = SimpleNote.where { it.title == "Goodbye" }

        assertTrue(results.isEmpty())
    }

    // =========================================================================
    // Delete Tests
    // =========================================================================

    @Test
    fun `delete marks record as deleted`() {
        val note = SimpleNote(title = "Delete me")
        note.save()

        note.delete()

        assertEquals(SyncState.DELETED, note.syncState)
    }

    @Test
    fun `delete removes record from all results`() {
        val note = SimpleNote(title = "Delete me")
        note.save()

        assertEquals(1, SimpleNote.all().size)

        note.delete()

        assertEquals(0, SimpleNote.all().size)
    }

    @Test
    fun `delete removes record from find`() {
        val note = SimpleNote(title = "Delete me")
        note.save()
        val id = note.id

        note.delete()

        assertNull(SimpleNote.find(id))
    }

    // =========================================================================
    // Serialization Tests
    // =========================================================================

    @Test
    fun `toJson includes all fields`() {
        val note = SimpleNote(title = "Test", content = "Content")
        note.save()

        val json = note.toJson()

        assertEquals(note.id, json["id"]?.toString()?.trim('"'))
        assertEquals("Test", json["title"]?.toString()?.trim('"'))
        assertEquals("Content", json["content"]?.toString()?.trim('"'))
    }

    @Test
    fun `fromJson restores all fields`() {
        val original = SimpleNote(title = "Original", content = "Text")
        original.save()

        val json = original.toJson()
        val restored = ActiveRecord.fromJson<SimpleNote>(json)

        assertEquals(original.id, restored.id)
        assertEquals(original.title, restored.title)
        assertEquals(original.content, restored.content)
    }

    // =========================================================================
    // UserId Tests
    // =========================================================================

    @Test
    fun `personal record gets userId from context`() {
        val note = SimpleNote(title = "Test")

        assertEquals("test-user", note.userId)
    }

    @Test
    fun `userId is included in json`() {
        val note = SimpleNote(title = "Test")
        note.save()

        val json = note.toJson()

        assertEquals("test-user", json["user_id"]?.toString()?.trim('"'))
    }
}

// =============================================================================
// Test Model
// =============================================================================

@PersonalBucket("simple-notes")
class SimpleNote(
    var title: String = "",
    var content: String = ""
) : PersonalActiveRecord() {

    companion object {
        fun find(id: String): SimpleNote? = ActiveRecord.find(id)
        fun all(): List<SimpleNote> = ActiveRecord.all()
        fun where(predicate: (SimpleNote) -> Boolean): List<SimpleNote> = ActiveRecord.where(predicate)
    }
}
