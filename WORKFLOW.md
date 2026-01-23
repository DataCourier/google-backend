# Complete Setup & Deployment Workflow

## How It Works

### The Portability Model

This backend uses a **copy-paste deployment model**:

1. **During development**: Edit `/shared` code (single source of truth)
2. **Before deployment**: Run `./setup.sh` to sync `/shared` → each service
3. **Each service is self-contained**: All dependencies copied in
4. **For new app**: Copy entire `/backend` folder, edit `config.yaml`, run `./setup.sh`

### File Flow

```
/shared/auth/firebase.go
      ↓ (setup.sh)
/services/user-service/auth/firebase.go  (copied)
/services/org-service/auth/firebase.go   (copied)
```

Each service gets its own copy. No import path coupling. True isolation.

---

## Initial Setup (This Project)

You're already set up! But here's what happened:

```bash
# 1. Created config.yaml
cat config.yaml

# 2. Ran setup to sync shared code
./setup.sh

# 3. Verify structure
tree services/user-service
# services/user-service/
# ├── auth/         ← copied from /shared
# ├── middleware/   ← copied from /shared
# ├── response/     ← copied from /shared
# ├── main.go
# └── go.mod        ← updated with module path
```

---

## Copy to New Project Workflow

When you want to create backend for a new mobile app:

### Step 1: Copy the template
```bash
# From your projects directory
cp -r backend my-new-app-backend
cd my-new-app-backend
```

### Step 2: Edit config.yaml
```yaml
project:
  name: "my-new-app"
  firebase_project_id: "my-new-app-prod"  # Your Firebase project
  module_path: "github.com/yourusername/my-new-app-backend"
```

### Step 3: Run setup
```bash
./setup.sh
```

This will:
- Copy `/shared` code into each service
- Update `go.mod` files with your new module path
- Prepare services for deployment

### Step 4: Deploy
```bash
# Deploy to Cloud Run
make deploy

# Or manually
cd services/user-service
gcloud run deploy user-service --source . --region us-central1 --allow-unauthenticated
```

Done! Completely isolated backend for your new app.

---

## Development Workflow

### Editing Shared Code

```bash
# 1. Edit shared utilities
nano shared/auth/firebase.go

# 2. Sync to all services
make sync
# or: ./setup.sh

# 3. Test locally (if Go installed)
cd services/user-service
go run main.go

# 4. Deploy
make deploy-user
```

### Adding New Shared Code

```bash
# 1. Create new utility in /shared
nano shared/database/firestore.go

# 2. Sync to services
./setup.sh

# 3. Use in service
# Import: "github.com/.../services/user-service/database"
```

### Adding New Service

```bash
# 1. Create service directory
mkdir -p services/payment-service

# 2. Create main.go and go.mod
cat > services/payment-service/go.mod << EOF
module github.com/yourusername/my-app-backend/services/payment-service
go 1.23
EOF

cat > services/payment-service/main.go << EOF
package main
// ... your service code
EOF

# 3. Add to config.yaml
# services:
#   - name: payment-service
#     enabled: true
#     port: 8082

# 4. Run setup to sync shared code
./setup.sh

# 5. Deploy
cd services/payment-service
gcloud run deploy payment-service --source . --region us-central1
```

---

## Current Status

### ✅ What's Working

- Config-driven setup (`config.yaml`)
- Shared code syncing (`./setup.sh`)
- User service structure
- Middleware (logging, CORS, recovery)
- Response helpers
- Firebase auth utilities (structure only, not tested)
- Cloud Run deployment ready

### 🚧 TODO

- [ ] Add Firebase dependencies to go.mod
- [ ] Test Firebase auth integration
- [ ] Implement org-service
- [ ] Add Firestore integration
- [ ] Create deployment CI/CD (optional)
- [ ] Write tests
- [ ] Add environment-specific config (dev/staging/prod)

---

## Design Decisions Summary

### Why separate go.mod per service?
- Cloud Run deploys from service directory (`--source .`)
- True isolation between services
- Aligns with "copy-paste backend" model
- No import path coupling

### Why copy /shared instead of Go modules?
- Simplifies copy-paste workflow
- No module path rewriting needed
- Each service 100% self-contained
- Clear about duplication (not hiding it)

### Why single config.yaml?
- Single source of truth
- Easy to update for new project
- Setup script reads it and does everything

### Why not Dockerfile?
- Cloud Run auto-detects Go and builds it
- One less file to maintain
- Can add later if needed for custom builds

---

## Next Steps

1. **Test local deployment** (requires Go installed)
2. **Deploy to Cloud Run** (no local Go needed)
3. **Add Firebase integration** (Firestore + Auth)
4. **Implement org-service**
5. **Test full copy-paste workflow** with a second project

---

## Questions?

See:
- [README.md](./README.md) - Quick start
- [backend-architecture.md](./backend-architecture.md) - Architecture details
- [PORTABILITY.md](./PORTABILITY.md) - Design philosophy
