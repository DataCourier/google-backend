# Offline-First Active Record SDK - Build Plan

**Goal:** True Active Record pattern with offline-first architecture for KMP (Android/iOS)

## Current State vs. Target State

### Current (Repository Pattern)
```kotlin
val notes = client.repository<Note>()
val note = Note().apply { title = "Hello" }
notes.save(note)  // Repository saves
notes.find(id)    // Repository finds
```

### Target (True Active Record)
```kotlin
val note = Note(title = "Hello")
note.save()      // Record saves itself
Note.find(id)    // Class method finds
Note.all()       // Class method lists all

// Works offline - syncs when online
// User ID assigned on first app launch (before sign-up)
```

---

## Core Architecture

### 1. Device Identity (Pre-Auth User)

Every device gets a **permanent anonymous ID** on first launch:

```kotlin
// Generated once, stored in secure storage (Keychain/EncryptedSharedPrefs)
val deviceUserId = "device_${UUID.randomUUID()}"

// Used for all local operations
// When user signs up/in → link device data to real account
```

**Flow:**
```
1. App installed → Generate device_user_id
2. User works offline → All data tagged with device_user_id
3. User signs up → Backend links device_user_id to real user_id
4. All historical data now belongs to real user
```

### 2. Client-Side UUID Generation

**All IDs generated on client** (from FRAMEWORK-ARCHITECTURE.md):

```kotlin
val session = ElevatorSession(
    id = UUID.randomUUID(),  // Generated client-side
    startTime = Clock.System.now()
)
```

**Why UUIDs?**
- No server round-trip for ID assignment
- No ID conflicts on sync
- No ID remapping needed
- Works offline

### 3. Dirty Flag & Sync Queue

```kotlin
// Local record state
enum class SyncState {
    SYNCED,      // Matches server
    DIRTY,       // Created/modified locally, needs push
    DELETED,     // Marked for deletion on server
    CONFLICT     // Server has different version
}

// Every record tracks its state
abstract class Record {
    var id: String = UUID.randomUUID().toString()
    var syncState: SyncState = SyncState.DIRTY
    var localUpdatedAt: Instant = Clock.System.now()
    var serverUpdatedAt: Instant? = null
}
```

### 4. Storage Layers

```
┌─────────────────────────────────────────────────────────────┐
│                    Active Record API                         │
│                 note.save() / Note.find(id)                 │
├─────────────────────────────────────────────────────────────┤
│                    Local Storage                             │
│         JSON in SharedPreferences / UserDefaults            │
├─────────────────────────────────────────────────────────────┤
│                    Sync Manager                              │
│         Push dirty → Server, Pull changes ← Server          │
├─────────────────────────────────────────────────────────────┤
│                    HTTP Client                               │
│                   Bucket API calls                           │
└─────────────────────────────────────────────────────────────┘
```

---

## Phased Build Plan

### Phase 1: True Active Record Pattern (Local Only)

**Goal:** `note.save()` works locally without network

**Files to create:**
```
src/commonMain/kotlin/com/example/buckets/
├── ActiveRecord.kt      # Base class with save/delete/find
├── JsonStorage.kt       # JSON in SharedPrefs/UserDefaults
├── BucketContext.kt     # Global config (storage, user, sync)
└── DeviceIdentity.kt    # Persistent device user ID
```

**ActiveRecord.kt:**
```kotlin
@OptIn(ExperimentalUuidApi::class)
abstract class ActiveRecord {
    var id: String = Uuid.random().toString()
    var syncState: SyncState = SyncState.DIRTY
    var createdAt: Instant = Clock.System.now()
    var updatedAt: Instant = Clock.System.now()

    // Instance methods
    suspend fun save(): Result<Unit> {
        updatedAt = Clock.System.now()
        syncState = SyncState.DIRTY
        return BucketContext.storage.save(this)
    }

    suspend fun delete(): Result<Unit> {
        syncState = SyncState.DELETED
        return BucketContext.storage.delete(this)
    }

    val isPersisted: Boolean get() = syncState != SyncState.DIRTY ||
        BucketContext.storage.exists(this)

    // Class methods via companion object
    companion object {
        // Subclasses override these
        inline fun <reified T : ActiveRecord> find(id: String): Result<T> {
            return BucketContext.storage.find(T::class, id)
        }

        inline fun <reified T : ActiveRecord> all(): Result<List<T>> {
            return BucketContext.storage.all(T::class)
        }

        inline fun <reified T : ActiveRecord> where(
            predicate: (T) -> Boolean
        ): Result<List<T>> {
            return BucketContext.storage.where(T::class, predicate)
        }
    }
}
```

**Usage:**
```kotlin
@PersonalBucket("notes")
class Note(
    var title: String = "",
    var content: String = ""
) : PersonalActiveRecord() {

    companion object : ActiveRecordCompanion<Note>(Note::class)
}

// Create and save
val note = Note(title = "Shopping", content = "Milk, eggs")
note.save()  // Saves locally, marked dirty

// Find
val loaded = Note.find(note.id)

// Query
val pinned = Note.where { it.isPinned }

// Delete
note.delete()
```

**DeviceIdentity.kt:**
```kotlin
object DeviceIdentity {
    private const val KEY = "bucket_device_user_id"

    // Generated once on first access, persisted forever
    val userId: String by lazy {
        SecureStorage.get(KEY) ?: run {
            val id = "device_${Uuid.random()}"
            SecureStorage.set(KEY, id)
            id
        }
    }

    // After real sign-in
    var linkedUserId: String? = null

    // Effective user ID for API calls
    val effectiveUserId: String
        get() = linkedUserId ?: userId
}
```

**Deliverables:**
- [ ] ActiveRecord base class with save/delete
- [ ] PersonalActiveRecord / OrgActiveRecord subclasses
- [ ] Companion object pattern for find/all/where
- [ ] JsonStorage (platform expect/actual for SharedPrefs/UserDefaults)
- [ ] DeviceIdentity with secure persistent ID
- [ ] BucketContext for global configuration
- [ ] Unit tests (local only, no network)

---

### Phase 2: Sync Manager

**Goal:** Automatic push/pull when online

**Files to add:**
```
src/commonMain/kotlin/com/example/buckets/
├── SyncManager.kt       # Coordinates sync operations
├── SyncQueue.kt         # Queue of pending operations
├── ConflictResolver.kt  # Handle conflicts
└── NetworkMonitor.kt    # Detect online/offline
```

**SyncManager.kt:**
```kotlin
class SyncManager(
    private val client: BucketClient,
    private val storage: LocalStorage
) {
    private val syncQueue = SyncQueue()

    // Push all dirty records to server
    suspend fun pushDirty(): SyncResult {
        val dirty = storage.getDirty()

        for (record in dirty) {
            try {
                when (record.syncState) {
                    SyncState.DIRTY -> pushRecord(record)
                    SyncState.DELETED -> deleteRemote(record)
                    else -> { /* skip */ }
                }
            } catch (e: ConflictException) {
                handleConflict(record, e.serverVersion)
            }
        }

        return SyncResult(pushed = dirty.size)
    }

    // Pull changes from server (last sync timestamp)
    suspend fun pull(): SyncResult {
        val lastSync = storage.getLastSyncTimestamp()
        val changes = client.getChanges(since = lastSync)

        for (change in changes) {
            storage.mergeFromServer(change)
        }

        return SyncResult(pulled = changes.size)
    }

    // Full sync
    suspend fun sync(): SyncResult {
        return pushDirty() + pull()
    }
}
```

**Auto-sync triggers:**
```kotlin
// In BucketContext initialization
BucketContext.configure {
    // Sync when network becomes available
    onNetworkAvailable { syncManager.sync() }

    // Sync on app foreground
    onAppForeground { syncManager.sync() }

    // Sync after save (debounced)
    afterSave { record ->
        syncManager.queueSync(debounce = 5.seconds)
    }
}
```

**Conflict Resolution:**
```kotlin
sealed class ConflictStrategy {
    object ServerWins : ConflictStrategy()
    object ClientWins : ConflictStrategy()
    object LastWriteWins : ConflictStrategy()
    class Custom(val resolver: (local: Record, server: Record) -> Record) : ConflictStrategy()
}

// Per-bucket configuration
@PersonalBucket("notes", conflictStrategy = ConflictStrategy.LastWriteWins)
class Note : PersonalActiveRecord()
```

**Deliverables:**
- [ ] SyncManager with push/pull
- [ ] SyncQueue for batching operations
- [ ] NetworkMonitor (expect/actual for platform)
- [ ] ConflictResolver with strategies
- [ ] Auto-sync triggers
- [ ] Sync status callbacks for UI

---

### Phase 3: Account Linking

**Goal:** Link device data to real user on sign-up/sign-in

**Flow:**
```
Device User (offline) → Sign Up → Link → Real User (synced)

1. device_abc123 creates notes offline
2. User signs up → gets user_xyz789
3. Backend: UPDATE records SET user_id = 'user_xyz789'
            WHERE user_id = 'device_abc123'
4. Client: DeviceIdentity.linkedUserId = 'user_xyz789'
5. All future syncs use real user_id
```

**Backend endpoint:**
```go
// POST /auth/link-device
func linkDeviceHandler(w http.ResponseWriter, r *http.Request) {
    realUserID := r.Context().Value("user_id").(string)
    deviceUserID := r.FormValue("device_user_id")

    // Transfer all device data to real user
    err := transferOwnership(deviceUserID, realUserID)
    if err != nil {
        respondError(w, err)
        return
    }

    respondJSON(w, 200, map[string]string{
        "message": "Device linked successfully",
        "user_id": realUserID,
    })
}
```

**Client:**
```kotlin
// After magic link verification
client.verifyMagicLink(token).onSuccess { session ->
    // Link device data to real account
    client.linkDevice(DeviceIdentity.userId).onSuccess {
        DeviceIdentity.linkedUserId = session.userId
        syncManager.sync() // Push any remaining dirty data
    }
}
```

**Deliverables:**
- [ ] Backend /auth/link-device endpoint
- [ ] Ownership transfer in Firestore
- [ ] Client linkDevice() method
- [ ] Handle already-linked devices
- [ ] Migration of offline data

---

### Phase 4: Real-time Sync (Optional)

**Goal:** Firestore listeners for collaborative features

**Only for specific buckets that need it:**
```kotlin
@OrgBucket("shared-docs", realtime = true)
class SharedDoc : OrgActiveRecord() {
    // Changes sync in real-time via Firestore
}

@PersonalBucket("notes", realtime = false)  // default
class Note : PersonalActiveRecord() {
    // Standard push/pull sync
}
```

---

## API Summary

### Initialization
```kotlin
// App startup
BucketContext.configure {
    baseUrl = "https://api.example.com"
    storage = JsonStorage(context)
    autoSync = true
    conflictStrategy = ConflictStrategy.LastWriteWins
}
```

### CRUD Operations
```kotlin
// Create
val note = Note(title = "Hello", content = "World")
note.save()  // Local + queued for sync

// Read
val note = Note.find("uuid-123")
val all = Note.all()
val pinned = Note.where { it.isPinned }

// Update
note.title = "Updated"
note.save()

// Delete
note.delete()
```

### Auth Flow
```kotlin
// Works immediately (offline-capable)
val note = Note(title = "My first note")
note.save()  // Saved with device_user_id

// Later, user signs up
BucketContext.requestMagicLink("alice@example.com")
// User clicks link...
BucketContext.verifyMagicLink(token).onSuccess {
    // Device data linked to real account
    // All notes now belong to alice
}
```

### Sync Control
```kotlin
// Manual sync
BucketContext.sync()

// Check sync status
BucketContext.syncStatus.collect { status ->
    when (status) {
        is SyncStatus.Synced -> showGreenDot()
        is SyncStatus.Syncing -> showSpinner()
        is SyncStatus.Offline -> showOfflineIndicator()
        is SyncStatus.Error -> showError(status.message)
    }
}

// Pending changes count
val pending = BucketContext.pendingChangesCount
```

---

## Implementation Order

| Phase | Effort | Dependency | Value |
|-------|--------|------------|-------|
| Phase 1: Active Record Local | 2-3 days | None | High - Core pattern |
| Phase 2: Sync Manager | 2-3 days | Phase 1 | High - Online/offline |
| Phase 3: Account Linking | 1 day | Phase 2 | High - Real users |
| Phase 4: Real-time (optional) | 2 days | Phase 2 | Low - Specific use cases |

**Recommended:** Phase 1 → 2 → 3 (core functionality in ~1 week)

---

## Key Design Decisions

### 1. Why client-side UUIDs?
- No server round-trip for ID
- Works offline
- No ID conflicts on sync
- No remapping needed

### 2. Why device identity before sign-up?
- Immediate value (app works before account)
- Lower friction (try before sign up)
- No data loss on sign up
- Seamless transition to real account

### 3. Why dirty flag vs. event sourcing?
- Simpler mental model
- Works for most CRUD apps
- Event sourcing adds complexity
- Can upgrade later if needed

### 4. Why JSON storage only?
- Zero dependencies
- Fast for small datasets (1000s of records)
- Built into platform (SharedPrefs/UserDefaults)
- Filter/sort in memory is instant for typical app sizes

---

## Testing Strategy

### Unit Tests (Phase 1)
```kotlin
class NoteTest {
    @Test
    fun `save persists locally`() = runTest {
        val note = Note(title = "Test")
        note.save()

        val loaded = Note.find(note.id).getOrThrow()
        assertEquals("Test", loaded.title)
    }

    @Test
    fun `new record is dirty`() {
        val note = Note()
        assertEquals(SyncState.DIRTY, note.syncState)
    }
}
```

### Integration Tests (Phase 2)
```kotlin
class SyncTest {
    @Test
    fun `dirty records sync when online`() = runTest {
        // Create offline
        val note = Note(title = "Offline note")
        note.save()

        // Go online
        networkMonitor.setOnline(true)
        syncManager.sync()

        // Verify on server
        val serverNote = client.getPersonal("notes", note.id)
        assertEquals("Offline note", serverNote["title"])
    }
}
```

---

## Next Steps

1. **Approve this plan**
2. **Start Phase 1** - Refactor current SDK to true Active Record
3. **Add DeviceIdentity** - Persistent anonymous user
4. **Test locally** - Unit tests for local operations
5. **Phase 2** - Add sync manager
6. **End-to-end test** - Full offline → online → sync flow
