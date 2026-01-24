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

##### P2P Sharing (via sharings table)

```
Data stays in owner's personal bucket
Sharing is a pointer, not a copy
Owner retains full control
Revoke = delete the sharing record
```

**Use case:** Share a note with a friend, collaborative doc between individuals

**How it works:**
```
┌──────────────────┐       ┌──────────────────┐
│ personal-notes   │       │ sharings         │
│ (user-a's data)  │◄──────│                  │
│                  │       │ owner: user-a    │
│ id: abc123       │       │ shared_with: b   │
│ title: "Hi"      │       │ item_id: abc123  │
│                  │       │ access: read     │
└──────────────────┘       └──────────────────┘
```

**API:**
```
POST /sharing/notes/{id}         # share with someone
DELETE /sharing/notes/{id}/{user} # unshare
GET /sharing/with-me             # what's shared with me
GET /sharing/by-me               # what I've shared
GET /sharing/notes/{id}          # access shared item
PUT /sharing/notes/{id}          # update (if write access)
```

---

##### Org Bucket (for companies/teams)

**Philosophy: The org owns everything. Users are temporary.**

```
Everything created belongs to the org, not the user
No cross-org data bleed - hard boundary enforced
Users come and go, data stays with org
created_by is metadata, not ownership
Access via org membership + roles, not individual permissions
```

**Key Principles:**

1. **Org is the owner**
   - User creates a doc → org owns it
   - `created_by: user-a` is just audit metadata
   - User can't take it with them

2. **Hard org boundary**
   - Can't share org data outside the org
   - Can't accidentally leak to personal bucket
   - Org ID is on every document, enforced at framework level

3. **Users are secondary**
   - Access = org membership + role
   - User leaves org → instant loss of access
   - New hire + role → instant access to relevant data

4. **Roles, not individual permissions**
   - `admin`, `member`, `viewer`, etc.
   - Role determines what you can do
   - No per-document permission management

**Data structure:**
```go
// Every org bucket document has:
{
  "id": "doc-123",
  "org_id": "acme-corp",      // HARD REQUIREMENT - enforced
  "created_by": "user-a",     // audit trail only
  "created_at": "...",
  "data": { ... }
}
```

**Access check:**
```go
// Framework enforces:
1. User must be member of org_id
2. User's role must permit the action
3. No exceptions, no workarounds
```

**Use case:** Company wikis, team projects, org-wide settings, employee data

**Example:**
```
POST /buckets/org/acme-corp/projects
{
  "name": "Q1 Launch",
  "status": "active"
}

// Framework automatically:
// - Verifies user is member of acme-corp
// - Sets org_id = acme-corp (can't override)
// - Sets created_by = current user
// - Stores in org-projects collection
```

**User leaves org:**
```
Before: user-a is member of acme-corp, can access all org data
After:  user-a removed from acme-corp, zero access instantly
Data:   unchanged, still in org, new hire can access it
```

---

#### Org Bucket Specification

##### Org Roles (hierarchy)

```
owner    → full control, can delete org, transfer ownership
admin    → manage members, all resources, settings
manager  → manage resources, invite users
member   → create/edit resources based on visibility
guest    → read-only access to explicitly shared resources
```

Role permissions cascade down:
```
owner > admin > manager > member > guest
```

##### Org Membership Table

```yaml
org-members:
  - org_id: acme-corp
    user_id: user-a
    role: owner
    invited_by: null
    joined_at: ...

  - org_id: acme-corp
    user_id: user-b
    role: member
    invited_by: user-a
    joined_at: ...

  - org_id: acme-corp
    user_id: user-c
    role: guest
    invited_by: user-b
    joined_at: ...
```

##### Resource Visibility (the "shareable" mixin)

Every org resource can have a visibility mode:

```
private      → only creator can see
invite-only  → creator + explicitly invited users/roles
team         → all members+ can see (not guests)
org-wide     → everyone in org including guests
```

When a resource is created, it gets a default visibility based on bucket config:

```go
// Bucket declares default visibility
{Name: "projects", Type: OrgBucket, DefaultVisibility: "team"}
{Name: "drafts", Type: OrgBucket, DefaultVisibility: "private"}
{Name: "announcements", Type: OrgBucket, DefaultVisibility: "org-wide"}
```

##### Org Sharings Table

Like P2P sharing, but scoped to org. Controls invite-only access:

```yaml
org-sharings:
  - org_id: acme-corp
    resource_type: projects    # bucket name
    resource_id: proj-123

    # WHO can access (one of these):
    shared_with_user: user-c   # specific user
    shared_with_role: manager  # everyone with this role+

    access: read               # read or write
    shared_by: user-a
    created_at: ...
```

##### Access Resolution (framework enforced)

```
Can user X access resource R in org O?

1. Is user X a member of org O?
   NO → deny (hard boundary)

2. What is resource R's visibility?

   "org-wide" →
     allow if member/guest of org

   "team" →
     allow if role >= member

   "invite-only" →
     check org-sharings table:
       - shared_with_user = X? allow
       - shared_with_role <= X's role? allow
     else: allow if X is creator

   "private" →
     allow only if X is creator

3. What access level?
   - Check org-sharings for read/write
   - role >= manager can always write team/org-wide resources
   - creator can always write their own
```

##### Resource Schema (automatic via mixin)

Every shareable org resource gets these fields automatically:

```go
{
  "id": "proj-123",

  // Org boundary (REQUIRED, enforced)
  "org_id": "acme-corp",

  // Audit trail
  "created_by": "user-a",
  "created_at": "...",
  "updated_at": "...",

  // Visibility (from mixin)
  "visibility": "invite-only",  // private|invite-only|team|org-wide

  // Actual data
  "name": "Q1 Launch",
  "status": "active",
  ...
}
```

##### Bucket Config with Abilities

```go
var OrgBuckets = []OrgBucketConfig{
    {
        Name: "projects",
        DefaultVisibility: "team",
        Abilities: []string{"shareable", "commentable"},
    },
    {
        Name: "documents",
        DefaultVisibility: "private",
        Abilities: []string{"shareable", "versionable"},
    },
    {
        Name: "announcements",
        DefaultVisibility: "org-wide",
        Abilities: []string{},  // read-only for most, no sharing needed
    },
}
```

Abilities are mixins that add behavior:
- `shareable` → adds visibility field, enables org-sharings
- `commentable` → enables comments sub-collection
- `versionable` → enables version history

##### API Examples

```bash
# Create project (visibility defaults to "team")
POST /org/acme-corp/buckets/projects
{"name": "Q1 Launch"}

# Make it invite-only
PUT /org/acme-corp/buckets/projects/proj-123
{"visibility": "invite-only"}

# Share with specific user
POST /org/acme-corp/sharing/projects/proj-123
{"user_id": "user-c", "access": "read"}

# Share with role (all managers can see)
POST /org/acme-corp/sharing/projects/proj-123
{"role": "manager", "access": "write"}

# List what's shared with me in this org
GET /org/acme-corp/sharing/with-me

# Guest user-c can only see:
# - org-wide resources
# - resources explicitly shared with them
```

##### The Mental Model

```
┌─────────────────────────────────────────────────────────┐
│  Org: acme-corp                                         │
│  ┌─────────────────────────────────────────────────┐   │
│  │  owner: user-a                                   │   │
│  │  admins: [user-b]                               │   │
│  │  managers: [user-c, user-d]                     │   │
│  │  members: [user-e, user-f, ...]                 │   │
│  │  guests: [contractor-1, client-1]               │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  Resources:                                             │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐   │
│  │ proj-123     │ │ doc-456      │ │ announce-1   │   │
│  │ vis: team    │ │ vis: private │ │ vis: org-wide│   │
│  │ → members+   │ │ → creator    │ │ → everyone   │   │
│  └──────────────┘ └──────────────┘ └──────────────┘   │
│                                                         │
│  Org Sharings:                                          │
│  ┌─────────────────────────────────────────────────┐   │
│  │ proj-123 → guest:contractor-1 (read)            │   │
│  │ doc-456 → role:manager (write)                  │   │
│  └─────────────────────────────────────────────────┘   │
│                                                         │
│  NOTHING LEAVES THIS BOX                                │
└─────────────────────────────────────────────────────────┘
```

---

##### Future Buckets (as patterns emerge)

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

// apps/acme-crm/buckets/config.go
var Buckets = []BucketConfig{
    {Name: "contacts", Type: OrgBucket},      // org owns all contacts
    {Name: "deals", Type: OrgBucket},         // org owns all deals
    {Name: "user-settings", Type: PersonalBucket}, // user's own prefs
}
```

The framework handles:
- Permission checks (user ownership / org membership + role)
- ID attachment (user_id / org_id)
- CRUD operations
- Boundary enforcement (no cross-org bleed)
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
