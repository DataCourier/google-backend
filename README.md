# Google Backend Template

**Copy-paste backend services for mobile apps built with Go, Cloud Run, and Firestore.**

Zero hosting fees for low-traffic apps. No multi-tenant complexity. Each app gets its own isolated backend.

> Built by [@DataCourer](https://github.com/DataCourer)

---

## 🚀 Quick Start

```bash
# 1. Copy this repo
git clone https://github.com/DataCourer/google-backend.git my-app-backend
cd my-app-backend

# 2. Edit config
nano config.yaml  # Set your Firebase project ID

# 3. Setup & deploy
./setup.sh
make deploy
```

**That's it.** Your backend is live on Cloud Run.

**New here?** Start with [how-to-create-new-app.md](./how-to-create-new-app.md) - shows you the simple workflow without the architectural details.

## Quick Start (New Project)

```bash
# 1. Copy this entire folder to your new project
cp -r backend my-new-app-backend
cd my-new-app-backend

# 2. Edit config.yaml with your Firebase project ID
nano config.yaml

# 3. Run setup (syncs shared code, updates module paths)
./setup.sh

# 4. Deploy to Cloud Run
make deploy
```

## Architecture

See [backend-architecture.md](./backend-architecture.md) for full details.
See [PORTABILITY.md](./PORTABILITY.md) for deployment strategy.

## Project Structure

```
/backend
  config.yaml          # Single source of truth for project config
  setup.sh             # Syncs shared code, updates modules
  Makefile             # Deploy, test, sync commands

  /shared              # Infrastructure only (no domain logic!)
    /auth              # Firebase token verification
    /middleware        # Logging, CORS, recovery
    /response          # JSON helpers
    /database          # Firestore client setup
    /validation        # Generic validators (isValidEmail)

  /services            # Each service is a complete vertical slice
    /user-service
      go.mod           # Independent module
      main.go
      /models          # User model (domain logic, NEVER shared)
      /handlers        # HTTP handlers
      /auth            # ← Copied from /shared during setup
      /middleware      # ← Copied from /shared during setup
      /response        # ← Copied from /shared during setup
      /database        # ← Copied from /shared during setup
      /validation      # ← Copied from /shared during setup
```

**Key Rule:** Models and domain logic are always per-service. Only infrastructure goes in `/shared`.

## Development Workflow

### Making changes to shared code
```bash
# 1. Edit files in /shared
nano shared/auth/firebase.go

# 2. Sync to all services
make sync

# 3. Test locally
cd services/user-service && go run main.go

# 4. Deploy
make deploy
```

### Adding a new service
```bash
# 1. Add to config.yaml
# 2. Create service directory
mkdir -p services/my-service
# 3. Run setup to copy shared code
./setup.sh
```

## Commands

```bash
make setup      # Run initial setup (calls setup.sh)
make sync       # Sync /shared code into all services
make deploy     # Deploy all enabled services to Cloud Run
make test       # Run tests for all services
```

## Requirements

- gcloud CLI configured with your project
- Go 1.23+ (optional, Cloud Run builds it)
- Firebase project created

## 💰 Cost

With proper free tier usage:
- Cloud Run: ~$0/month (scale to zero)
- Firestore: ~$0/month (under 50k reads/day)
- Firebase Auth: ~$0/month
- Cloud Build: ~$0/month (120 build-minutes/day free)

Total: **$0/month** for low-traffic apps.

---

## 📚 Documentation

- **[how-to-create-new-app.md](./how-to-create-new-app.md)** - Start here (5 min guide)
- **[DEPLOYMENT.md](./DEPLOYMENT.md)** - Complete deployment guide
- **[WORKFLOW.md](./WORKFLOW.md)** - Development workflows
- **[backend-architecture.md](./backend-architecture.md)** - Architecture & tech decisions
- **[DATABASE.md](./DATABASE.md)** - Models, validation, Firestore patterns
- **[PORTABILITY.md](./PORTABILITY.md)** - Design philosophy

See [DOCS.md](./DOCS.md) for full documentation index.

---

## ✨ Features

- ✅ **Portable** - Copy-paste to create new app backends
- ✅ **Free tier friendly** - Scale to zero, generous free tiers
- ✅ **No auth complexity** - Firebase Auth handles magic links
- ✅ **Type safe** - Go on backend, define contracts for mobile
- ✅ **Fast cold starts** - Go on Cloud Run (~50-100ms)
- ✅ **Isolated** - Each app independent, no multi-tenancy
- ✅ **Simple deployment** - One command, auto-builds

---

## 🎮 Example: Tic-Tac-Toe Game

Includes a complete working example (`game-service`) that demonstrates:
- Database writes (create game)
- Database reads (get game state)
- Database updates (make moves)
- Validation (turn-based, position checking)
- Game logic (win detection)
- Token-based access (UUIDs)
- HTML templates

**Try it live:** [Deploy instructions](./services/game-service/README.md)

---

## 🛠️ Tech Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| Runtime | Go 1.23 | Fast cold starts, type safety, stdlib power |
| Hosting | Cloud Run | Scale to zero, free tier, auto-builds |
| Database | Firestore | Document DB, free tier, real-time |
| Auth | Firebase Auth | Magic links, OAuth, no email service needed |
| Templates | html/template | Stdlib, no dependencies |

---

## 📖 Services Included

### User Service
Thin wrapper around Firebase Auth for custom claims and metadata.

### Game Service
Complete tic-tac-toe game with URL-based multiplayer. Great learning example.

### Org Service *(coming soon)*
Organization/team management with membership roles.

---

## 🤝 Contributing

Issues and PRs welcome! This is a template repo - feel free to fork and customize for your needs.

---

## 📄 License

MIT - Use freely for personal or commercial projects.

---

## 🙏 Credits

Built with ☕ and inspiration from:
- Google Cloud Platform
- Firebase
- The Go community
