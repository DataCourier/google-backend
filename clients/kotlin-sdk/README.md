# Bucket SDK for Kotlin Multiplatform

Active Record-inspired SDK for the Bucket API. Works on Android, iOS, and JVM.

## Installation

Add to your `build.gradle.kts`:

```kotlin
dependencies {
    implementation("com.example:bucket-sdk:0.1.0")
}
```

## Quick Start

### 1. Define Your Models

```kotlin
// Personal bucket - user-scoped data
@PersonalBucket("notes")
class Note : PersonalRecord() {
    var title: String = ""
    var content: String = ""
    var tags: List<String> = emptyList()
}

// Org bucket - organization-scoped with roles
@OrgBucket("projects")
class Project : OrgRecord() {
    var name: String = ""
    var description: String = ""
    var status: String = "active"
}
```

### 2. Initialize Client

```kotlin
val client = BucketClient("https://api.example.com")

// For development (no auth needed)
client.setAuthToken("local:my-user-id")

// For production (magic link auth)
client.requestMagicLink("alice@example.com")
// User clicks email link...
client.verifyMagicLink(tokenFromUrl).onSuccess { session ->
    client.setAuthToken(session.token)
}
```

### 3. CRUD Operations

```kotlin
val notes = client.repository<Note>()

// Create
val note = Note().apply {
    title = "Hello"
    content = "World"
}
notes.save(note).onSuccess { println("ID: ${it.id}") }

// Read
notes.find(note.id!!).onSuccess { loaded ->
    println(loaded.title)
}

// Update
note.title = "Updated"
notes.save(note) // Detects existing ID, does update

// Delete
notes.delete(note)
```

### 4. Org Buckets

```kotlin
// Specify org ID when creating repository
val projects = client.repository<Project>(orgId = "acme-corp")

// Create with visibility
val project = Project().apply {
    name = "Secret Project"
    visibility = Visibility.INVITE_ONLY
}
projects.save(project)

// List all visible to user
projects.all().onSuccess { list ->
    list.forEach { println(it.name) }
}
```

## Model Annotations

| Annotation | Target | Description |
|------------|--------|-------------|
| `@PersonalBucket("name")` | Class | User-scoped bucket |
| `@OrgBucket("name")` | Class | Org-scoped bucket |
| `@Id` | Property | Primary key (auto-generated) |
| `@ReadOnly` | Property | Server-managed field |
| `@Field("json_name")` | Property | Custom JSON field name |

## Base Classes

### PersonalRecord

```kotlin
abstract class PersonalRecord : Record {
    var id: String?           // Auto-generated
    val createdAt: Instant?   // Server-set
    val updatedAt: Instant?   // Server-set
    val userId: String?       // Server-set (your user ID)
}
```

### OrgRecord

```kotlin
abstract class OrgRecord : Record {
    var id: String?
    val createdAt: Instant?
    val updatedAt: Instant?
    val orgId: String?        // Server-enforced
    val createdBy: String?    // Creator's user ID
    var visibility: String    // private, invite-only, team, org-wide
}
```

## Visibility Modes

| Mode | Who Can See |
|------|-------------|
| `Visibility.PRIVATE` | Creator only |
| `Visibility.INVITE_ONLY` | Creator + explicitly shared |
| `Visibility.TEAM` | Members+ (not guests) |
| `Visibility.ORG_WIDE` | Everyone in org |

## Android ViewModel Example

```kotlin
class NotesViewModel(application: Application) : AndroidViewModel(application) {
    private val client = BucketClient("https://api.example.com").apply {
        setAuthToken(SessionManager.getToken(application))
    }
    private val repo = client.repository<Note>()

    private val _notes = MutableStateFlow<List<Note>>(emptyList())
    val notes: StateFlow<List<Note>> = _notes

    fun create(title: String, content: String) {
        viewModelScope.launch {
            repo.save(Note().apply {
                this.title = title
                this.content = content
            }).onSuccess { _notes.update { list -> list + it } }
        }
    }

    fun delete(note: Note) {
        viewModelScope.launch {
            repo.delete(note).onSuccess {
                _notes.update { list -> list.filter { it.id != note.id } }
            }
        }
    }
}
```

## iOS Usage

```swift
// From Swift, use the generated Kotlin framework
let client = BucketClient(baseUrl: "https://api.example.com")
client.setAuthToken(token: "local:my-user")

let notes = client.repository(recordClass: Note.self, orgId: nil)

// Async/await via Kotlin coroutines
Task {
    let note = Note()
    note.title = "Hello from iOS"

    let result = try await notes.save(record: note)
    print("Saved: \(result.id)")
}
```

## Error Handling

```kotlin
notes.save(note)
    .onSuccess { saved ->
        println("Saved: ${saved.id}")
    }
    .onFailure { error ->
        when (error) {
            is BucketException -> {
                when (error.statusCode) {
                    401 -> // Unauthorized - need to login
                    403 -> // Forbidden - no permission
                    404 -> // Not found
                    else -> // Server error
                }
            }
            else -> // Network error
        }
    }
```

## Local Development

Use `local:` tokens for development without magic link:

```kotlin
// Simple
client.setAuthToken("local:alice")

// With email and name
client.setAuthToken("local:alice:alice@test.com:Alice Smith")
```

The backend accepts these tokens when not in production mode.
