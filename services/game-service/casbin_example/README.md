# Casbin Example for Org Notes

Demonstrates Casbin RBAC with our org bucket model.

## Files

- `model.conf` - Casbin model definition (RBAC with domains/orgs)
- `policy.csv` - Permission rules and role assignments
- `example.go` - Runnable demo
- `integration.go` - How to integrate with org buckets

## Quick Start

```bash
cd casbin_example
go mod init casbin_example
go get github.com/casbin/casbin/v2
go run example.go
```

## Expected Output

```
=== Org Notes Permission Checks ===

✓ alice.notes.create = acme-corp
✗ alice.notes.approve = acme-corp
✓ alice.notes.view_published = acme-corp
✓ bob.notes.create = acme-corp
✓ bob.notes.approve = acme-corp
✗ bob.notes.publish = acme-corp
✓ bob.projects.create = acme-corp
✓ charlie.notes.publish = acme-corp
✓ charlie.notes.delete = acme-corp
✓ charlie.announcements.create = acme-corp
✗ eve.notes.create = acme-corp
✓ eve.notes.view_published = acme-corp
✗ alice.notes.create = other-corp
```

## How It Works

### Model (model.conf)

```
r = sub, org, resource, action    # Request: who, which org, what resource, what action
p = role, org, resource, action   # Policy: role can do action on resource in org
g = _, _, _                       # Grouping: user has role in org
```

### Policy Rules (policy.csv)

```
# role, org, resource, action
p, member, *, notes, create       # members can create notes (any org)
p, manager, *, notes, approve     # managers can approve
p, admin, *, notes, publish       # admins can publish

# user, role, org
g, alice, member, acme-corp       # alice is member of acme
g, bob, manager, acme-corp        # bob is manager of acme

# role inheritance
g, member, guest, *               # member inherits guest
g, manager, member, *             # manager inherits member
g, admin, manager, *              # admin inherits manager
```

### Check

```go
allowed, _ := e.Enforce("bob", "acme-corp", "notes", "approve")
// bob -> manager in acme-corp -> policy says manager can approve -> true
```

## vs Our Current System

| Current | Casbin |
|---------|--------|
| `if role == RoleGuest { return error }` | `if !authz.CanDo(user, org, "notes", "create")` |
| Hardcoded in Go | Config in CSV/DB |
| Role hierarchy in `roleLevel()` | Role inheritance in policy |
| Add action = change code | Add action = add policy line |

## Storage Options

Casbin supports many adapters:
- File (CSV, JSON)
- PostgreSQL, MySQL, MongoDB
- Redis
- Firestore (community adapter)
- In-memory with periodic sync

For our case, Firestore adapter would keep policies in sync.
