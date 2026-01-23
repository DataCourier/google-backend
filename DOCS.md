# Documentation Index

Start here, read what you need when you need it.

---

## Getting Started

### 1. [how-to-create-new-app.md](./how-to-create-new-app.md) ⭐ **START HERE**
How to create a new app backend in 5 minutes.
- Copy-paste workflow
- Day-to-day development
- Real-world examples
- FAQ

**Read this first if:** You just want to use it and don't care how it works.

---

## How-To Guides

### 2. [README.md](./README.md)
Quick reference and commands.
- Installation requirements
- Project structure
- Available commands (`make setup`, `make deploy`, etc.)
- Cost breakdown

**Read this when:** You need a quick command reference.

### 3. [DEPLOYMENT.md](./DEPLOYMENT.md) ⚠️ **IMPORTANT**
Complete deployment guide with troubleshooting.
- Prerequisites (GCP project, gcloud setup)
- Enable required APIs
- Create Firestore database
- Deploy services to Cloud Run
- Common issues & solutions
- Cost optimization tips

**Read this when:** You're deploying for the first time or troubleshooting deployment issues.

### 4. [WORKFLOW.md](./WORKFLOW.md)
Detailed step-by-step workflows.
- Initial setup (this project)
- Copy to new project workflow
- Development workflows (editing shared code, adding services)
- Current status checklist

**Read this when:** You need detailed steps for a specific task.

---

## Architecture & Design

### 5. [backend-architecture.md](./backend-architecture.md)
Technical architecture and tech stack.
- Why Go, Cloud Run, Firestore, Firebase Auth
- Data models (User, Org, Membership)
- Auth flow (magic link)
- API design principles
- Trade-offs and considerations

**Read this when:** You want to understand why technical decisions were made.

### 6. [PORTABILITY.md](./PORTABILITY.md)
Design philosophy and portability strategy.
- Goals (copy-paste backend, zero coupling, minimal friction)
- Architecture options analyzed
- Why we chose this approach
- Deployment workflow
- Open questions

**Read this when:** You want to understand the "why" behind the architecture.

### 7. [DATABASE.md](./DATABASE.md)
Models, validation, and Firestore patterns.
- Model structure (per-service vs shared)
- Validation strategy
- "Migrations" in Firestore (indexes, backfills, versioning)
- CRUD examples
- Testing with emulator

**Read this when:** You're implementing database features.

---

## Reading Order

### If you just want to use it:
1. [how-to-create-new-app.md](./how-to-create-new-app.md) - The simple workflow
2. [DEPLOYMENT.md](./DEPLOYMENT.md) - Deploy to Cloud Run
3. [README.md](./README.md) - Command reference
4. Done. Start building.

### If you want to understand it:
1. [how-to-create-new-app.md](./how-to-create-new-app.md) - What you do
2. [DEPLOYMENT.md](./DEPLOYMENT.md) - How to deploy
3. [backend-architecture.md](./backend-architecture.md) - Tech decisions
4. [PORTABILITY.md](./PORTABILITY.md) - Design philosophy
5. [WORKFLOW.md](./WORKFLOW.md) - Detailed workflows
6. [DATABASE.md](./DATABASE.md) - Implementation patterns

### If you're contributing or forking:
Read all of them, starting with PORTABILITY.md to understand the goals.

---

## Quick Links

**Commands:**
```bash
./setup.sh           # Sync shared code, update modules
make sync            # Sync shared code to services
make deploy          # Deploy all services
make deploy-user     # Deploy user-service only
make test            # Run tests
```

**Config:**
- `config.yaml` - Single source of truth for project configuration
- `firestore.indexes.json` - Firestore index definitions (TODO)
- `firestore.rules` - Security rules (TODO)

**Development:**
- `/shared` - Edit infrastructure code here
- `/services/user-service/models` - Edit domain models here
- `/services/user-service/handlers` - Edit HTTP handlers here

---

## TL;DR

**What is this?**
Copy-paste backend services for mobile apps. Each app gets its own isolated backend instance.

**Why would I use it?**
- Zero hosting fees (free tier)
- No multi-tenant complexity
- Breaking changes don't cascade across apps
- 5-minute setup for new apps

**How do I use it?**
```bash
cp -r backend my-app
cd my-app
nano config.yaml    # Edit project config
./setup.sh          # Setup
make deploy         # Deploy
```

**Where do I start?**
[how-to-create-new-app.md](./how-to-create-new-app.md)
