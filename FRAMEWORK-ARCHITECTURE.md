# Backend/Mobile Framework Architecture

**Intent-Driven Architecture for Rapid Prototyping**

---

## Core Philosophy

Move away from MVC/REST's form-driven, implicit behavior model to an **intent-driven architecture** where data persistence and actions are explicitly separated.

**Traditional REST/MVC:**
- Implicit actions via data changes
- CRUD endpoints per table
- Resource nesting (`/users/123/articles/456`)
- Controllers + Models + Service objects
- Callbacks/observers for side effects
- Server infers intent from state changes

**Our Framework:**
- Explicit actions as first-class concept
- Reusable bucket types
- Flat modules by function
- Buckets + Actions + Triggers
- Explicit action calls
- Client declares intent explicitly

---

## Three Primitives

### 1. Buckets (Data Persistence)

**Purpose:** Handle all data storage with built-in permission models

**Behavior:**
- Pure persistence
- No side effects
- No business logic
- **Universal & Reusable:** Same bucket types work across all apps

#### Bucket Types

##### Personal Bucket (default)

```
User owns all their data
Backend auto-attaches user_id
Can write any structure (schema-less for prototyping)
Cannot overwrite other users' data
```

**Use case:** User preferences, private documents, personal game state

**Example:**
```go
// User writes to their personal bucket
POST /buckets/personal/elevator-sessions
{
  "session_id": "uuid-123",
  "start_time": "2026-01-24T10:00:00Z",
  "floor_count": 5
}

// Backend automatically:
// - Attaches user_id from auth token
// - Ensures user can only read/write their own data
// - No permission logic needed in app code
```

##### Shared Bucket (for collaboration)

```
Documents have permission metadata
Multiple users can write based on access rules
Permission checks at document level
```

**Use case:** Team documents, multiplayer games, collaborative boards

**Example:**
```go
// Create shared document with permissions
POST /buckets/shared/game-rooms
{
  "room_id": "uuid-456",
  "owner": "user-123",
  "players": ["user-123", "user-789"],
  "permissions": {
    "read": ["user-123", "user-789"],
    "write": ["user-123", "user-789"]
  }
}
```

##### Future Buckets (as patterns emerge)

- **Org-level bucket:** Company-wide data
- **Public bucket:** Publicly readable, admin writable
- **Immutable bucket:** Write-once, read-many (audit logs)

#### Key Insight

**Instead of implementing CRUD endpoints per table, implement bucket behaviors once.**

Apps just declare which bucket type they use:

```go
// apps/gentle-motion/buckets/config.go
var Buckets = []BucketConfig{
    {Name: "elevator-sessions", Type: PersonalBucket},
    {Name: "user-preferences", Type: PersonalBucket},
}
```

The framework handles:
- Permission checks
- User ID attachment
- CRUD operations
- Validation
- Querying

---

### 2. Actions (Explicit Behaviors)

**Purpose:** All business logic, workflows, side effects

**Organized by:** Module/functional area, NOT resource hierarchy

**Explicit user intent:** User taps "Publish" → calls `publish` action

#### Structure

```
/apps/gentle-motion/actions/
  elevator/
    start_session.go
    end_session.go
    export_data.go
  user/
    update_preferences.go
    send_welcome_email.go
  analytics/
    generate_report.go
```

#### What Actions Can Do

- Send emails
- Process data
- Mutate multiple resources
- Trigger external services
- Return computed results
- Complex business logic

#### Example

```go
// apps/gentle-motion/actions/elevator/start_session.go
func StartSession(ctx context.Context, userID string, req StartSessionRequest) (*Session, error) {
    // 1. Create session in personal bucket
    session := Session{
        ID:        uuid.New(),
        UserID:    userID,
        StartTime: time.Now(),
        Status:    "active",
    }

    // 2. Save to personal bucket
    if err := buckets.Personal.Write(ctx, "elevator-sessions", session.ID, session); err != nil {
        return nil, err
    }

    // 3. Send notification (side effect)
    notifications.Send(userID, "Session started!")

    // 4. Log analytics (side effect)
    analytics.Track("session_started", userID)

    return &session, nil
}
```

#### Key Insight

**Separates "save data" from "do something"**

No more inferring intent from data changes. Client explicitly calls the action:

```kotlin
// Mobile app
button.onClick {
    api.actions.elevator.startSession()
}
```

Not:
```kotlin
// DON'T DO THIS (implicit intent)
session.status = "active"
session.save() // What happens here? Who knows!
```

---

### 3. Triggers (Server-side Automation)

**Purpose:** Time-based or event-based automation

#### Types

##### Cron Triggers

**Use case:** Scheduled tasks, periodic checks

```go
// apps/gentle-motion/triggers/daily_summary.go
func init() {
    triggers.RegisterCron("daily-summary", "0 9 * * *", func(ctx context.Context) error {
        // Run every day at 9am
        users := getAllActiveUsers()
        for _, user := range users {
            summary := generateDailySummary(user)
            sendEmail(user.Email, summary)
        }
        return nil
    })
}
```

**Examples:**
- Daily digest emails
- Weekly report generation
- Cleanup old sessions
- Check for pending publications

##### Event Triggers (future)

**Use case:** React to data changes, external events

```go
triggers.RegisterEvent("on-session-complete", func(ctx context.Context, session Session) error {
    // Automatically triggered when session status = "complete"
    report := generateSessionReport(session)
    buckets.Personal.Write(ctx, "session-reports", report.ID, report)
    return nil
})
```

#### Key Insight

Handles "publish this tomorrow" without:
- Client-side scheduling
- Implicit callbacks
- Background workers per feature

---

## Architecture Stack

### Backend (Go on Cloud Run)

```
/apps/
  gentle-motion/
    buckets/      # which bucket types this app uses
      config.go   # bucket declarations
    actions/      # app-specific behaviors
      elevator/
        start_session.go
        end_session.go
      user/
        update_preferences.go
    triggers/     # scheduled/automated tasks
      daily_summary.go
      cleanup_old_data.go

  tic-tac-toe/
    buckets/
      config.go   # uses shared bucket for game rooms
    actions/
      game/
        create_room.go
        make_move.go
    triggers/
      cleanup_abandoned_games.go
```

**Benefits:**
- ~100ms response time
- Near-zero cost at low scale
- Infinite scaling
- Full control over logic
- Centralized logging

---

### Mobile (Kotlin Multiplatform)

#### Client-side Architecture

```kotlin
// Models declare storage strategy via annotations
@GoBackend  // default - hits Go API
data class ElevatorSession(
    val id: UUID,
    val startTime: Instant,
    val floorCount: Int
)

@FirebaseBackend  // optional - uses Firestore for real-time
data class ChatMessage(
    val id: UUID,
    val sender: String,
    val text: String,
    val timestamp: Instant
)
```

#### Offline-first Flow

1. **User works in app** (offline or online)
2. **Changes marked "dirty"** in local cache
3. **When user signs in** → sync dirty data to backend
4. **UUIDs prevent conflicts** (no ID remapping needed)

```kotlin
// User creates session offline
val session = ElevatorSession(
    id = UUID.randomUUID(),  // Generated client-side
    startTime = Clock.System.now(),
    floorCount = 5
)

// Saved to local cache with "dirty" flag
localCache.save(session, dirty = true)

// Later, when online:
syncManager.syncDirtyData() // Pushes to backend
```

#### Storage Options

**Simple apps:** JSON in SharedPreferences/UserDefaults

```kotlin
// Lightweight local persistence
class LocalStorage {
    fun save(key: String, data: Any) {
        prefs.putString(key, Json.encodeToString(data))
    }
}
```

**Queries needed:** SQLDelight

```kotlin
// When you need to query/filter locally
database.elevatorSessionQueries
    .selectByDateRange(startDate, endDate)
    .executeAsList()
```

**Real-time needed:** Firestore (bolt-on for specific features)

```kotlin
// Only for features that need real-time sync
@FirebaseBackend
data class LiveGameState(...)
```

---

## Traditional Approach vs. Our Framework

| Traditional Approach | Your Framework |
|---------------------|----------------|
| Implicit actions via data changes | Explicit actions as first-class concept |
| CRUD endpoints per table | Reusable bucket types |
| Resource nesting `/users/123/articles/456` | Flat modules by function |
| Controllers + Models + Service objects | Buckets + Actions + Triggers |
| Callbacks/observers for side effects | Explicit action calls |
| Server infers intent from state changes | Client declares intent explicitly |

---

## API Examples

### Traditional REST (Bad)

```
POST /users/123/sessions
PUT /users/123/sessions/456
DELETE /users/123/sessions/456

Problem: What happens when you PUT?
- Email sent?
- Analytics tracked?
- Other users notified?
- Who knows! Hidden in controller code.
```

### Our Framework (Good)

```
# Data persistence (buckets)
POST   /buckets/personal/sessions          # Save data
GET    /buckets/personal/sessions/:id      # Read data
PUT    /buckets/personal/sessions/:id      # Update data
DELETE /buckets/personal/sessions/:id      # Delete data

# Explicit behaviors (actions)
POST /actions/elevator/start-session       # Start session + side effects
POST /actions/elevator/end-session         # End session + generate report
POST /actions/elevator/export-data         # Export as PDF + email

Clear: Each action does exactly what it says.
```

---

## Evolution Path

### Phase 1: (Now)

- [ ] Build Personal Bucket implementation
- [ ] Build basic action pattern
- [ ] Get one app (tic-tac-toe or gentle-motion) working end-to-end
- [ ] Document bucket + action patterns

### Phase 2:

- [ ] Add Shared Bucket when collaboration needed
- [ ] Add cron triggers for scheduling
- [ ] Contract endpoint for version negotiation
- [ ] Offline sync implementation

### Phase 3:

- [ ] Firebase integration for real-time (bolt-on)
- [ ] Cross-app pattern reuse
- [ ] Dynamic client code generation from contract
- [ ] Additional bucket types as patterns emerge

---

## The Big Win

**You've solved the boring problems once:**
- Permissions
- Persistence
- Offline sync

**Now you only write:**
- **Actions:** Unique business logic per app
- **Data models:** Just declare which bucket

**Everything else is reusable infrastructure.**

---

## No More...

❌ Per-table CRUD boilerplate
❌ Hidden callbacks
❌ Guessing what happens when you save data
❌ Resource nesting hell
❌ Implicit behavior via observers

---

## Just...

✅ **Buckets** for data
✅ **Actions** for behavior
✅ **Triggers** for automation

**Clean. Explicit. Reusable.**

---

## Next Steps

1. Read `BUCKETS.md` for bucket implementation details
2. Read `ACTIONS.md` for action patterns
3. Read `TRIGGERS.md` for scheduling/automation
4. Start migrating tic-tac-toe to use this pattern
5. Build gentle-motion app using framework

---

## Questions?

This framework is designed for:
- **Rapid prototyping:** Schema-less buckets, quick iteration
- **Clean architecture:** Explicit intent, no hidden behavior
- **Code reuse:** Same patterns across all apps
- **Scaling:** Start simple, add complexity when needed

Not designed for:
- Complex legacy migrations
- Apps that need GraphQL
- Real-time everything (use Firebase bolt-on when needed)
