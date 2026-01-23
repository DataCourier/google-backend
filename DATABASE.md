# Database, Models, Validation & Migrations

## Database Layer (Firestore)

### No Traditional Migrations

Firestore is a document DB, not SQL. No schema enforcement, no migrations like Postgres/MySQL.

**What you DO need:**
- ✅ Firestore indexes (`firestore.indexes.json`)
- ✅ Security rules (`firestore.rules`)
- ✅ Go structs for type safety
- ✅ Validation logic
- ✅ Schema versioning strategy

**What you DON'T need:**
- ❌ SQL migrations (no ALTER TABLE)
- ❌ Schema enforcement (Firestore is schemaless)

---

## Architecture Decisions

### 1. Where do models live?

**Answer: Always per-service. Never shared.**

```
/services/user-service/models/user.go          ← User service owns User
/services/org-service/models/org.go            ← Org service owns Org
/services/org-service/models/membership.go     ← Org service owns Membership
```

**Why:**
- Each service owns its domain models
- True service isolation
- Can evolve independently
- No coupling between services

**What about duplication?**
- If two services need the same type, duplicate it
- Prefer isolation over DRY
- Example: Both services might have a "User" concept, but they're different contexts

### What goes in `/shared`?

**Only infrastructure/utilities:**
- ✅ Firestore client setup (boilerplate)
- ✅ Middleware (logging, CORS, recovery)
- ✅ Response helpers (JSON formatting)
- ✅ Auth utilities (token verification)
- ✅ Generic validators (`isValidEmail`)
- ✅ Transaction helpers

**Never domain logic:**
- ❌ Models (User, Org, etc.)
- ❌ Business rules
- ❌ Service-specific logic

**Rule of thumb:** If it knows about your domain (users, orgs, payments), it's per-service. If it's generic infrastructure, it's shared.

---

## 2. Model Structure

### Go Structs with Firestore Tags

```go
// services/user-service/models/user.go
package models

import (
    "time"
    "cloud.google.com/go/firestore"
)

type User struct {
    ID        string                 `firestore:"id" json:"id"`
    Email     string                 `firestore:"email" json:"email"`
    CreatedAt time.Time              `firestore:"createdAt" json:"createdAt"`
    UpdatedAt time.Time              `firestore:"updatedAt" json:"updatedAt"`
    Metadata  map[string]interface{} `firestore:"metadata" json:"metadata,omitempty"`
}

// Collection name
const UsersCollection = "users"

// Save creates or updates a user
func (u *User) Save(ctx context.Context, client *firestore.Client) error {
    u.UpdatedAt = time.Now()
    if u.CreatedAt.IsZero() {
        u.CreatedAt = u.UpdatedAt
    }

    _, err := client.Collection(UsersCollection).Doc(u.ID).Set(ctx, u)
    return err
}

// GetByID fetches a user by ID
func GetUserByID(ctx context.Context, client *firestore.Client, id string) (*User, error) {
    doc, err := client.Collection(UsersCollection).Doc(id).Get(ctx)
    if err != nil {
        return nil, err
    }

    var user User
    if err := doc.DataTo(&user); err != nil {
        return nil, err
    }

    return &user, nil
}

// List users with pagination
func ListUsers(ctx context.Context, client *firestore.Client, limit int, startAfter string) ([]*User, error) {
    query := client.Collection(UsersCollection).Limit(limit).OrderBy("createdAt", firestore.Desc)

    if startAfter != "" {
        doc, err := client.Collection(UsersCollection).Doc(startAfter).Get(ctx)
        if err != nil {
            return nil, err
        }
        query = query.StartAfter(doc)
    }

    docs, err := query.Documents(ctx).GetAll()
    if err != nil {
        return nil, err
    }

    users := make([]*User, len(docs))
    for i, doc := range docs {
        var u User
        doc.DataTo(&u)
        users[i] = &u
    }

    return users, nil
}
```

---

## 3. Validation

### Input Validation Strategy

**Option A: Struct tags + validator library**
```go
import "github.com/go-playground/validator/v10"

type CreateUserRequest struct {
    Email string `json:"email" validate:"required,email"`
    Name  string `json:"name" validate:"required,min=2,max=100"`
}

var validate = validator.New()

func (r *CreateUserRequest) Validate() error {
    return validate.Struct(r)
}
```

**Option B: Manual validation (Recommended for simplicity)**
```go
type CreateUserRequest struct {
    Email string `json:"email"`
    Name  string `json:"name"`
}

func (r *CreateUserRequest) Validate() error {
    if r.Email == "" {
        return errors.New("email is required")
    }
    if !isValidEmail(r.Email) {
        return errors.New("email is invalid")
    }
    if len(r.Name) < 2 || len(r.Name) > 100 {
        return errors.New("name must be 2-100 characters")
    }
    return nil
}
```

**Recommendation:** Start with **manual validation**. Add validator library later if needed.

### Where to validate?

```go
// In handler
func createUserHandler(w http.ResponseWriter, r *http.Request) {
    var req CreateUserRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        response.BadRequest(w, "Invalid JSON")
        return
    }

    // Validate
    if err := req.Validate(); err != nil {
        response.BadRequest(w, err.Error())
        return
    }

    // Business logic...
}
```

---

## 4. Migrations (Firestore Context)

### What "migrations" mean for Firestore:

**1. Index Management**
```json
// firestore.indexes.json
{
  "indexes": [
    {
      "collectionGroup": "memberships",
      "queryScope": "COLLECTION",
      "fields": [
        {"fieldPath": "userId", "order": "ASCENDING"},
        {"fieldPath": "active", "order": "ASCENDING"}
      ]
    }
  ]
}
```

Deploy with: `firebase deploy --only firestore:indexes`

**2. Data Backfills (when schema changes)**

Example: Adding `displayName` field to existing users

```go
// scripts/backfill-display-name.go
package main

func main() {
    ctx := context.Background()
    client, _ := firestore.NewClient(ctx, "project-id")

    users, _ := client.Collection("users").Documents(ctx).GetAll()

    for _, doc := range users {
        // Add missing field
        client.Collection("users").Doc(doc.Ref.ID).Update(ctx, []firestore.Update{
            {Path: "displayName", Value: extractNameFromEmail(doc.Data()["email"])},
        })
    }
}
```

Run manually or as Cloud Function.

**3. Schema Versioning**

Add version field to documents:

```go
type User struct {
    ID        string    `firestore:"id"`
    Email     string    `firestore:"email"`
    Version   int       `firestore:"version"`  // Schema version
    // ...
}

// On read, check version and migrate if needed
func GetUser(ctx context.Context, client *firestore.Client, id string) (*User, error) {
    doc, _ := client.Collection("users").Doc(id).Get(ctx)
    var user User
    doc.DataTo(&user)

    if user.Version < 2 {
        // Migrate v1 -> v2
        user.DisplayName = extractNameFromEmail(user.Email)
        user.Version = 2
        user.Save(ctx, client)
    }

    return &user, nil
}
```

---

## 5. Proposed Structure

```
/backend
  /shared                              ← Infrastructure only (no domain logic)
    /database
      client.go                        # Firestore client setup (boilerplate)
      helpers.go                       # Generic transaction helpers

    /validation
      email.go                         # isValidEmail helper
      helpers.go                       # Generic validators

  /services/user-service               ← User service owns all user domain logic
    /models
      user.go                          # User model + CRUD + validation

    /handlers
      create_user.go                   # HTTP handlers
      get_user.go
      update_user.go

    main.go

  /services/org-service                ← Org service owns org domain logic
    /models
      org.go                           # Org model
      membership.go                    # Membership model

    /handlers
      create_org.go
      add_member.go

    main.go

  /scripts                             ← Operational scripts
    /migrations
      001-backfill-display-name.go

  /deploy                              ← Firestore config
    firestore.indexes.json
    firestore.rules
```

**Key principle:** Each service is a complete vertical slice. Models live where they're used.

---

## 6. Firestore Client Setup

### Shared database client

```go
// shared/database/client.go
package database

import (
    "context"
    "cloud.google.com/go/firestore"
)

// NewClient creates a new Firestore client
func NewClient(ctx context.Context, projectID string) (*firestore.Client, error) {
    return firestore.NewClient(ctx, projectID)
}

// Transaction helpers
func RunTransaction(ctx context.Context, client *firestore.Client, fn func(context.Context, *firestore.Transaction) error) error {
    return client.RunTransaction(ctx, fn)
}
```

---

## 7. Example: Full CRUD for User

```go
// services/user-service/models/user.go
package models

import (
    "context"
    "errors"
    "time"

    "cloud.google.com/go/firestore"
)

type User struct {
    ID        string                 `firestore:"id" json:"id"`
    Email     string                 `firestore:"email" json:"email"`
    CreatedAt time.Time              `firestore:"createdAt" json:"createdAt"`
    UpdatedAt time.Time              `firestore:"updatedAt" json:"updatedAt"`
    Metadata  map[string]interface{} `firestore:"metadata,omitempty" json:"metadata,omitempty"`
}

const UsersCollection = "users"

// Create
func CreateUser(ctx context.Context, client *firestore.Client, email string) (*User, error) {
    user := &User{
        ID:        "", // Firebase Auth UID, or generate
        Email:     email,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
        Metadata:  make(map[string]interface{}),
    }

    ref := client.Collection(UsersCollection).NewDoc()
    user.ID = ref.ID

    if _, err := ref.Set(ctx, user); err != nil {
        return nil, err
    }

    return user, nil
}

// Read
func GetUser(ctx context.Context, client *firestore.Client, id string) (*User, error) {
    doc, err := client.Collection(UsersCollection).Doc(id).Get(ctx)
    if err != nil {
        return nil, err
    }

    var user User
    if err := doc.DataTo(&user); err != nil {
        return nil, err
    }

    return &user, nil
}

// Update
func UpdateUser(ctx context.Context, client *firestore.Client, id string, updates map[string]interface{}) error {
    updates["updatedAt"] = time.Now()

    _, err := client.Collection(UsersCollection).Doc(id).Set(ctx, updates, firestore.MergeAll)
    return err
}

// Delete
func DeleteUser(ctx context.Context, client *firestore.Client, id string) error {
    _, err := client.Collection(UsersCollection).Doc(id).Delete(ctx)
    return err
}

// List with pagination
func ListUsers(ctx context.Context, client *firestore.Client, limit int) ([]*User, error) {
    docs, err := client.Collection(UsersCollection).
        Limit(limit).
        OrderBy("createdAt", firestore.Desc).
        Documents(ctx).GetAll()

    if err != nil {
        return nil, err
    }

    users := make([]*User, len(docs))
    for i, doc := range docs {
        var u User
        doc.DataTo(&u)
        users[i] = &u
    }

    return users, nil
}
```

---

## 8. Testing Strategy

```go
// models/user_test.go
package models

import (
    "context"
    "testing"

    "cloud.google.com/go/firestore"
)

func TestCreateUser(t *testing.T) {
    ctx := context.Background()
    client := setupTestFirestore(t) // Use emulator

    user, err := CreateUser(ctx, client, "test@example.com")
    if err != nil {
        t.Fatal(err)
    }

    if user.Email != "test@example.com" {
        t.Errorf("expected test@example.com, got %s", user.Email)
    }
}
```

Use Firestore emulator for testing:
```bash
firebase emulators:start --only firestore
export FIRESTORE_EMULATOR_HOST=localhost:8080
go test ./...
```

---

## Questions to Decide

1. **Validation library or manual?** (Recommend: start manual)
2. **Models per-service or shared?** (Recommend: per-service)
3. **Schema versioning strategy?** (Add `version` field? Handle on read or background job?)
4. **Soft deletes?** (Add `deletedAt` field? Or hard delete?)
5. **Pagination strategy?** (Cursor-based with Firestore's `StartAfter`?)

---

## Next Steps

- [ ] Create `/shared/database/client.go`
- [ ] Create `/services/user-service/models/user.go`
- [ ] Add Firestore dependencies to `go.mod`
- [ ] Create `/deploy/firestore.indexes.json`
- [ ] Create `/deploy/firestore.rules`
- [ ] Write model tests with emulator

Want me to implement this?
