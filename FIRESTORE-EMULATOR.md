# Firestore Emulator Setup

Fast local development with isolated test data.

---

## Why Use the Emulator?

**Without emulator:**
- 🐌 2-3s startup (network roundtrip to GCP)
- ⚠️ Writes to production database
- 🌐 Requires internet

**With emulator:**
- ⚡ ~100ms startup (local)
- 🔒 Isolated test data
- ✈️ Works offline
- 🚨 Server errors if emulator not running (safety!)

---

## One-Time Setup

Install the emulator:

```bash
gcloud components install cloud-firestore-emulator
```

Verify:

```bash
gcloud emulators firestore --help
```

---

## Daily Workflow

**Terminal 1: Start Emulator** (keep running)

```bash
make emulator
```

Or manually:

```bash
gcloud emulators firestore start
```

You should see:
```
[firestore] API endpoint: http://localhost:8080
```

**Terminal 2: Run Service**

```bash
make dev-game
```

Or manually:

```bash
cd services/game-service
export PATH=$HOME/go/bin:$PATH
export GCP_PROJECT=michal-playground-2026
export FIRESTORE_EMULATOR_HOST=localhost:8080
go run .
```

**Terminal 3: Test**

```bash
curl http://localhost:8080/
curl -X POST http://localhost:8080/games/create
```

---

## Safety Feature: Required Emulator Check

The server **will not start** in dev mode without the emulator:

```go
if os.Getenv("ENV") != "production" {
    if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
        log.Fatal("🚨 ERROR: Firestore emulator not detected! ...")
    }
}
```

This prevents you from accidentally:
- Writing test data to production
- Hitting production Firestore during local dev
- Polluting prod with game rooms and debug data

---

## Production Deployment

Cloud Run deployments automatically use **real Firestore** (not emulator):

```bash
make deploy-game
```

The `ENV=production` check is automatically handled by Cloud Run environment.

---

## Template Hot Reload

While the server is running:

1. Edit `services/game-service/views/*.html`
2. Refresh browser (instant!)
3. No server restart needed

Only restart when changing `.go` files.

---

## Troubleshooting

**"Address already in use"**

The emulator uses port `8080`. If something else is using it:

```bash
# Find and kill process on port 8080
lsof -ti:8080 | xargs kill -9

# Or start emulator on different port
gcloud emulators firestore start --host-port=localhost:8081

# Then use:
export FIRESTORE_EMULATOR_HOST=localhost:8081
```

**"Emulator not detected" error**

Make sure:
1. Emulator is running in Terminal 1
2. `FIRESTORE_EMULATOR_HOST` is set in Terminal 2
3. You're running from project root or service directory

**Data persistence**

Emulator data is **in-memory** by default. When you stop the emulator, all data is lost.

This is GOOD for testing! Each session starts clean.

If you want persistent data:

```bash
gcloud emulators firestore start --host-port=localhost:8080 --rules=firestore.rules
```

---

## Summary

**Start emulator once per session:**
```bash
make emulator
```

**Run service (restarts on Go code changes):**
```bash
make dev-game
```

**Edit templates (no restart needed):**
- Edit HTML/CSS
- Refresh browser
- Instant feedback!

**Deploy when done testing:**
```bash
make deploy-game
```
