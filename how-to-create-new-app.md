# The Simple Part (What You Actually Do)

Despite all the architectural details in the other docs, the **user experience is dead simple**.

---

## For New App (Copy-Paste)

```bash
# 1. Copy the entire backend folder
cp -r backend my-new-app-backend
cd my-new-app-backend

# 2. Edit one config file
nano config.yaml
# Change:
#   - project name
#   - Firebase project ID
#   - module path (optional)

# 3. Run setup
./setup.sh

# 4. Deploy
make deploy
```

**Done.** Your new app has a complete, isolated backend.

**Time:** ~5 minutes

---

## For Development (Day-to-Day)

### Editing shared infrastructure code
```bash
nano shared/middleware/logging.go    # Edit shared code
make sync                            # Copies to all services
make deploy                          # Deploy
```

### Editing service-specific code
```bash
nano services/user-service/models/user.go    # Edit directly
cd services/user-service
go run main.go                               # Test locally (optional)
cd ../..
make deploy-user                             # Deploy just this service
```

### Adding a new endpoint
```bash
nano services/user-service/handlers/get_profile.go    # Create handler
nano services/user-service/main.go                    # Wire it up
make deploy-user                                       # Deploy
```

**That's it.** No complex builds, no orchestration, no configuration hell.

---

## The Complexity is Hidden (On Purpose)

Yes, there's architectural depth in the other docs:
- DATABASE.md - Models, validation, Firestore patterns
- PORTABILITY.md - Design decisions, trade-offs
- backend-architecture.md - Tech stack rationale

**But you don't need to understand all that to use it.**

### The architecture ensures:
- ✅ True portability (copy-paste just works)
- ✅ No cross-service dependencies
- ✅ Each copy is 100% independent
- ✅ Simple deployment (Cloud Run auto-builds)

### You just:
- ✅ Edit one config file
- ✅ Run one setup script
- ✅ Deploy with one command

---

## What Gets Copied

When you `cp -r backend my-new-app`:

```
backend/                    my-new-app-backend/
├── config.yaml      →      ├── config.yaml (edit this)
├── setup.sh         →      ├── setup.sh (run this)
├── Makefile         →      ├── Makefile (use this)
├── /shared          →      ├── /shared (infrastructure)
└── /services        →      └── /services (complete services)
```

Everything comes with you. Zero external dependencies. Zero coupling.

---

## What Happens During Setup

```bash
./setup.sh
```

**Behind the scenes:**
1. Reads your `config.yaml`
2. Copies `/shared` code into each service
3. Updates `go.mod` with your module path
4. Makes each service self-contained

**You see:**
```
🚀 Backend Setup Script
📋 Configuration:
   Project Name: my-new-app
   ...
✅ Setup complete!
```

**Result:** Each service is now a complete, independent Go application ready to deploy.

---

## What Happens During Deploy

```bash
make deploy
```

**Behind the scenes:**
1. Runs setup (syncs shared code)
2. Uploads service source to Cloud Run
3. Cloud Run detects Go, builds it
4. Deploys container
5. Returns URL

**You see:**
```
🚀 Deploying user-service...
Deploying container to Cloud Run service [user-service]...
✓ Deploying... Done.
  https://user-service-xyz-uc.a.run.app
```

**Result:** Your backend is live on a public URL.

---

## Example: Real Workflow

### Day 1: Create backend for "TaskApp"
```bash
cp -r backend taskapp-backend
cd taskapp-backend
nano config.yaml                    # Set project: taskapp-prod
./setup.sh
make deploy
# → https://user-service-xyz.run.app (live!)
```

### Day 30: Add profile photo feature
```bash
nano services/user-service/models/user.go      # Add PhotoURL field
nano services/user-service/handlers/upload.go  # Add upload handler
make deploy-user
# → Feature deployed
```

### Day 60: Create backend for "ShoppingApp" (reuse everything)
```bash
cp -r taskapp-backend ../shoppingapp-backend
cd ../shoppingapp-backend
nano config.yaml                    # Set project: shoppingapp-prod
./setup.sh
make deploy
# → Separate backend, zero coupling to TaskApp
```

**Each backend is 100% independent. Update TaskApp's backend without affecting ShoppingApp.**

---

## FAQ

### "Do I need to understand Go?"
Helps, but not required to get started. The templates are copy-paste ready.

### "Do I need to understand Firestore?"
Basic document DB concepts help. The models show you the patterns.

### "Do I need to understand the architecture docs?"
Nope. They explain *why* it works this way, but you can use it without reading them.

### "What if I break something?"
Each app is isolated. Breaking one backend doesn't affect others.

### "Can I use a different database?"
Yes, but you'd need to edit `/shared/database/` and models. Firestore is chosen for the free tier.

### "Can I use a different language?"
Sure, but you'd rewrite it. Go is chosen for cold start speed + Cloud Run efficiency.

### "What if I want multi-tenancy?"
Then this isn't for you. This architecture deliberately avoids multi-tenancy for simplicity.

---

## The Philosophy

**Simple operations, complex underneath (like a car):**
- You turn the key → engine does 1000 things
- You run `./setup.sh` → syncs, updates, prepares

**Copy-paste over microservice orchestration:**
- Each app gets its own isolated backend
- Breaking changes don't cascade
- Delete one app's backend without affecting others

**Free tier friendly:**
- Cloud Run scales to zero (no cost when idle)
- Firestore has generous free tier
- No always-on infrastructure

---

## Next Steps

See the other docs only when you need them:

- **README.md** - Quick reference, commands
- **WORKFLOW.md** - Detailed step-by-step workflows
- **backend-architecture.md** - Tech stack decisions (read if curious)
- **PORTABILITY.md** - Design philosophy (read if curious)
- **DATABASE.md** - Models and validation patterns (read when implementing features)

**Or just start coding.** The structure guides you.
