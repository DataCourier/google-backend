package com.example.buckets

import kotlinx.datetime.Instant

// =============================================================================
// EXAMPLE: Define your models
// =============================================================================

/**
 * Personal note - stored in user's personal bucket.
 */
@PersonalBucket("notes")
class Note : PersonalRecord() {
    var title: String = ""
    var content: String = ""
    var tags: List<String> = emptyList()
    var isPinned: Boolean = false
}

/**
 * Personal task - stored in user's personal bucket.
 */
@PersonalBucket("tasks")
class Task : PersonalRecord() {
    var title: String = ""
    var completed: Boolean = false
    var dueDate: Instant? = null
    var priority: Int = 0
}

/**
 * Org project - stored in org bucket with visibility.
 */
@OrgBucket("projects")
class Project : OrgRecord() {
    var name: String = ""
    var description: String = ""
    var status: String = "active"
    var members: List<String> = emptyList()
}

/**
 * Org announcement - stored in org bucket (org-wide by default).
 */
@OrgBucket("announcements")
class Announcement : OrgRecord() {
    var title: String = ""
    var body: String = ""
    var priority: String = "normal" // normal, important, urgent

    init {
        visibility = Visibility.ORG_WIDE
    }
}

// =============================================================================
// EXAMPLE: Usage
// =============================================================================

/**
 * Example usage of the Bucket SDK.
 */
suspend fun exampleUsage() {
    // Initialize client
    val client = BucketClient("http://localhost:8080")

    // -------------------------------------------------------------------------
    // Auth: Magic Link
    // -------------------------------------------------------------------------

    // Request magic link
    client.requestMagicLink("alice@example.com")
        .onSuccess { println("Check your email!") }
        .onFailure { println("Error: ${it.message}") }

    // After user clicks link, verify token (from URL param)
    val magicToken = "abc123..." // from /auth/verify?token=xxx
    client.verifyMagicLink(magicToken)
        .onSuccess { session ->
            println("Logged in as ${session.email}")
            client.setAuthToken(session.token)
        }

    // For local development, use local tokens:
    client.setAuthToken("local:alice:alice@example.com:Alice")

    // -------------------------------------------------------------------------
    // Personal Bucket: Notes
    // -------------------------------------------------------------------------

    val notes = client.repository<Note>()

    // Create a note
    val note = Note().apply {
        title = "Shopping List"
        content = "Milk, eggs, bread"
        tags = listOf("shopping", "groceries")
    }
    notes.save(note)
        .onSuccess { println("Saved note: ${it.id}") }
        .onFailure { println("Error: ${it.message}") }

    // Update the note
    note.isPinned = true
    notes.save(note) // Automatically updates since it has an ID

    // Find a note by ID
    notes.find(note.id!!)
        .onSuccess { loaded ->
            println("Found: ${loaded.title}")
        }

    // Delete the note
    notes.delete(note)

    // -------------------------------------------------------------------------
    // Org Bucket: Projects
    // -------------------------------------------------------------------------

    val projects = client.repository<Project>(orgId = "acme-corp")

    // Create a project (visible to team by default)
    val project = Project().apply {
        name = "Mobile App v2"
        description = "Next generation mobile app"
        status = "active"
        visibility = Visibility.TEAM
    }
    projects.save(project)
        .onSuccess { println("Created project: ${it.id}") }

    // List all visible projects
    projects.all()
        .onSuccess { list ->
            println("Found ${list.size} projects")
            list.forEach { println("  - ${it.name}") }
        }

    // Create org-wide announcement
    val announcements = client.repository<Announcement>(orgId = "acme-corp")
    val announcement = Announcement().apply {
        title = "Company Update"
        body = "We've reached 1M users!"
        priority = "important"
    }
    announcements.save(announcement)

    // -------------------------------------------------------------------------
    // Logout
    // -------------------------------------------------------------------------

    client.logout()
        .onSuccess { println("Logged out") }
}

// =============================================================================
// EXAMPLE: Android ViewModel usage
// =============================================================================

/*
class NotesViewModel : ViewModel() {
    private val client = BucketClient("https://api.example.com")
    private val notes = client.repository<Note>()

    private val _notes = MutableStateFlow<List<Note>>(emptyList())
    val notes: StateFlow<List<Note>> = _notes

    init {
        // Set auth token from saved session
        client.setAuthToken(SessionManager.getToken())
    }

    fun createNote(title: String, content: String) {
        viewModelScope.launch {
            val note = Note().apply {
                this.title = title
                this.content = content
            }
            notes.save(note)
                .onSuccess { savedNote ->
                    _notes.update { it + savedNote }
                }
                .onFailure { error ->
                    // Handle error
                }
        }
    }

    fun deleteNote(note: Note) {
        viewModelScope.launch {
            notes.delete(note)
                .onSuccess {
                    _notes.update { it.filter { n -> n.id != note.id } }
                }
        }
    }
}
*/
