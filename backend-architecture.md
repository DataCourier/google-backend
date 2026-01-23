# Backend Architecture

## Overview

Reusable, copy-paste backend services for mobile apps. Zero hosting fees tier, minimal friction.

## Tech Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Runtime | Go | 50-100ms cold starts, cheap compute, type safety |
| Hosting | Google Cloud Run | Scale to zero, free tier, no server management |
| Database | Firestore | Document DB, free tier, Firebase ecosystem |
| Auth | Firebase Auth | Built-in magic link (email link sign-in), OAuth, no extra email service |
| Email | Firebase (built-in) | Magic links handled by Firebase Auth, no verification fees |

## Deployment Model

- **One instance per app** (copy-paste, separate Firebase projects)
- No multi-tenant complexity
- Breaking changes don't cascade across apps

---

## Services

### 1. User Service

Thin wrapper around Firebase Auth. Firebase handles:
- Magic link generation & email delivery
- Google OAuth (when needed)
- Token issuance & refresh

User Service handles:
- Custom claims (roles, org membership)
- User metadata in Firestore
- Token verification for other services

### 2. Org Service

Organization/team management.

---

## Data Models

### User (`users/{uid}`)

```
{
  "id": "firebase-uid",
  "email": "user@example.com",
  "createdAt": timestamp,
  "updatedAt": timestamp,
  "metadata": { ... }      // app-specific only
}
```

### Org (`orgs/{id}`)

```
{
  "id": "firebase-generated",
  "name": "Acme Corp",
  "slug": "acme-corp",
  "createdAt": timestamp,
  "updatedAt": timestamp,
  "metadata": { ... }      // app-specific only
}
```

### Membership (`memberships/{id}`)

Join table for user-org relationships. Single source of truth.

```
{
  "id": "auto-generated",
  "userId": "firebase-uid",
  "orgId": "org-id",
  "role": "admin" | "member",
  "active": true,
  "joinedAt": timestamp,
  "updatedAt": timestamp,
  "metadata": { ... }      // app-specific only
}
```

**Firestore indexes needed:**
- `userId` (query: "all orgs for user X")
- `orgId` (query: "all members of org X")
- `orgId + role` (query: "all admins of org X")

### Slug Index (`slugs/{slug}`)

Enforces slug uniqueness (Firestore has no unique constraints).

```
{
  "orgId": "org-id"
}
```

Create slug doc in transaction with org creation. If slug exists, transaction fails.

---

## Auth Flow (Magic Link)

```
1. Mobile app → Firebase SDK: signInWithEmailLink(email)
2. Firebase → sends magic link email
3. User clicks link → deep link opens app (Android App Links / iOS Universal Links)
4. App → Firebase SDK: complete sign-in with link
5. App receives Firebase ID token
6. App → Backend: requests with ID token in Authorization header
7. Backend → verifies token via Firebase Admin SDK
```

---

## API Design Principles

- Pure REST API (no web frontend)
- Firebase ID token in `Authorization: Bearer <token>` header
- All documents have `metadata` field for app-specific flexibility
- Contracts defined on Kotlin side
- Services are stateless, all state in Firestore

---

## Considerations & Trade-offs

### Memberships as join table
- **Why**: Single source of truth, no dual-write consistency issues
- **Trade-off**: Extra query to get org members (vs embedded)
- **Worth it**: Cleaner, familiar pattern, queryable both directions

### Real columns vs metadata
- **Service fields**: Real columns (queryable, indexable, typed)
- **App-specific fields**: `metadata` object (flexible, no backend changes)
- **Rule**: If every app needs it → real column. If app-specific → metadata.

### Slug uniqueness
- **Problem**: Firestore has no unique constraints
- **Solution**: Separate `slugs` collection, transactional create

### Firestore indexes
- Create composite indexes for common query patterns
- `memberships`: userId, orgId, orgId+role
- Indexes defined in `firestore.indexes.json`, deployed with app

---

## Project Structure

Each service is self-contained. Copy what you need to a new project.

```
/backend
  /services
    /user-service
      main.go             # entrypoint, wires everything up
      handlers.go         # HTTP handlers
      models.go           # structs + Firestore ops
      /views              # optional: QR codes, occasional HTML, etc.
      /docs
        api.md            # API documentation

    /org-service
      main.go
      handlers.go
      models.go
      /views              # optional
      /docs
        api.md

  /shared                 # copied with services to new projects
    /auth
      firebase.go         # token verification
    /firestore
      client.go           # Firestore client setup
      helpers.go          # common query patterns
    /middleware
      logging.go
      cors.go
      recovery.go
    /response
      json.go             # standard JSON responses, errors

  /deploy
    firestore.indexes.json
    firestore.rules
    cloudbuild.yaml       # optional: CI/CD
```

**Copy-paste workflow for new app:**
1. Copy `/shared` folder
2. Copy needed services (`/user-service`, `/org-service`, etc.)
3. Copy `/deploy` config files
4. Update Firebase project ID
5. Deploy

**Deployment (Cloud Run source-based, no Dockerfile):**
```bash
cd services/user-service
gcloud run deploy user-service --source .
```
Cloud Run auto-detects Go and builds it. Add Dockerfile only if you need custom build control.

---

## Next Steps

1. [ ] Set up Go project structure
2. [ ] Firebase project setup
3. [ ] Implement User Service
4. [ ] Implement Org Service
5. [ ] Cloud Run deployment config
