# Portability & Deployment Strategy

## Goals

### Primary Goal: Copy-Paste Backend for New Apps

Each mobile app gets its own isolated backend instance. No multi-tenancy, no shared infrastructure, no cascading breaking changes.

**Workflow:**
1. Copy entire `/backend` folder to new project
2. Edit single config file with new app details (project name, Firebase project ID)
3. Run setup script
4. Deploy to Cloud Run
5. Done - fully isolated backend for new app

**Anti-goals:**
- ❌ Shared multi-tenant backend
- ❌ Complex orchestration or Kubernetes
- ❌ Framework lock-in or proprietary patterns
- ❌ Manual find-and-replace across multiple files

---

## Key Requirements

### 1. Zero Coupling Between App Instances
- Each backend copy is 100% independent
- Can update dependencies separately
- Breaking changes don't affect other apps
- Can delete an app's backend without affecting others

### 2. Minimal Setup Friction
- Single config file: `config.yaml` or similar
- One setup command: `./setup.sh` or `make setup`
- Auto-replace: project IDs, service names, Firebase config
- Idempotent (can re-run safely)

### 3. Free Tier Friendly
- Cloud Run scale-to-zero
- Firestore free tier (50k reads/day, 20k writes/day)
- Firebase Auth free tier
- No always-on infrastructure

### 4. Type Safety & Contracts
- Go on backend (compile-time safety)
- Kotlin on Android (shared data models)
- JSON contracts explicitly versioned

---

## Architecture Decisions

### Option A: Separate `go.mod` per Service (Current)

**Structure:**
```
/backend
  /services
    /user-service
      go.mod            # independent module
      main.go
      /auth
        firebase.go     # copied/vendored shared code
      /middleware
        logging.go      # copied/vendored shared code

    /org-service
      go.mod            # independent module
      main.go
      /auth
        firebase.go     # copied/vendored shared code
```

**Pros:**
- ✅ True isolation - each service is self-contained
- ✅ Cloud Run happy - deploys from service dir with `--source .`
- ✅ No import path coupling
- ✅ Can copy single service to new project

**Cons:**
- ❌ Duplicated shared code in each service
- ❌ Updates to shared code require manual sync

**Setup script would:**
- Run `find . -name go.mod -exec sed -i 's/OLD_PROJECT/NEW_PROJECT/g' {} \;`
- Update Firebase project ID in each service's config

---

### Option B: Single Root `go.mod` with Internal Packages

**Structure:**
```
/backend
  go.mod                # single module
  /services
    /user-service
      main.go
    /org-service
      main.go
  /internal
    /auth
      firebase.go       # shared
    /middleware
      logging.go        # shared
    /response
      json.go           # shared
```

**Pros:**
- ✅ No code duplication
- ✅ Single source of truth for shared code
- ✅ Standard Go monorepo pattern

**Cons:**
- ❌ Import paths: `github.com/youruser/backend/internal/auth`
- ❌ Setup script must update root module path
- ❌ Cloud Run deployment requires context from root

**Setup script would:**
- Update root `go.mod` module path
- Update all imports across all files
- More complex `gcloud run deploy` (build context at root)

---

### Option C: Hybrid - Template Expansion

**Structure:**
```
/backend-template
  config.template.yaml
  /services
    /user-service
      go.mod.template
      main.go
  /shared
    /auth
      firebase.go
  setup.sh
```

**Setup script:**
1. Reads `config.yaml` (created by user)
2. Expands all `.template` files with values
3. Copies `/shared` into each service
4. Generates final `go.mod` files with correct module paths

**Pros:**
- ✅ Best of both worlds - shared code during dev, isolated after setup
- ✅ Clear distinction between template and instance
- ✅ Can lint/test shared code before copying

**Cons:**
- ❌ More complex setup script
- ❌ "Template" concept might be overkill

---

## Proposed Solution: Option A with Vendoring Script

**Why:**
- Aligns with "copy-paste backend" philosophy
- Each service truly self-contained
- Simple Cloud Run deployment
- Explicit about duplication (not hiding it)

**Shared code strategy:**
- Keep canonical `/shared` directory during development
- `make sync` command copies shared code into each service before deployment
- Each service has its own copy in production
- Document which files are "vendored" from shared

**Config file: `backend.yaml`**
```yaml
project:
  name: "my-awesome-app"
  firebase_project_id: "my-awesome-app-prod"
  gcp_region: "us-central1"

services:
  - name: user-service
    port: 8080
  - name: org-service
    port: 8081

auth:
  magic_link_enabled: true
  google_oauth_enabled: true
```

**Setup script: `setup.sh`**
```bash
#!/bin/bash
# 1. Read backend.yaml
# 2. Update go.mod module paths in each service
# 3. Sync /shared code into each service
# 4. Generate .env files for local dev
# 5. Output Cloud Run deployment commands
```

---

## Deployment Workflow

### For New App
```bash
# 1. Copy template
cp -r backend-template my-app-backend
cd my-app-backend

# 2. Configure
cp backend.example.yaml backend.yaml
# Edit backend.yaml with your Firebase project ID

# 3. Setup
./setup.sh

# 4. Deploy
make deploy-all
# or individually:
cd services/user-service && gcloud run deploy user-service --source .
```

### For Updates to Shared Code
```bash
# Edit /shared/auth/firebase.go
make sync     # copies to all services
make deploy-all
```

---

## Questions to Answer

1. **Module naming convention?**
   - `github.com/username/PROJECT_NAME/services/user-service`?
   - `backend/services/user-service` (simpler, but assumes local-only)?

2. **How to handle Firebase credentials?**
   - Environment variables only?
   - Or allow `firebase-credentials.json` per service?

3. **Version strategy?**
   - Single VERSION file at root?
   - Or per-service versions?

4. **Makefile or shell script?**
   - `make setup`, `make sync`, `make deploy-all`?
   - Or `./setup.sh`, `./sync.sh`, `./deploy.sh`?

5. **Testing strategy?**
   - Tests in `/shared` that run before sync?
   - Tests in each service after sync?

---

## Next Steps

- [ ] Create `backend.yaml` schema
- [ ] Write `setup.sh` script
- [ ] Implement shared code sync mechanism
- [ ] Test full workflow: copy → setup → deploy
- [ ] Document in README.md
