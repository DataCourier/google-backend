# Local Development Setup

**Goal:** Test changes locally in ~2 seconds instead of waiting 2 minutes for Cloud Run deploys.

---

## Setup (One-Time)

### Step 1: Verify Go Installation

```bash
# Check if Go is installed
$HOME/go/bin/go version
# Should show: go version go1.23.5 linux/amd64

# If not installed, Go was already installed to ~/go during initial setup
# If you need to reinstall:
wget https://go.dev/dl/go1.23.5.linux-amd64.tar.gz
tar -C $HOME -xzf go1.23.5.linux-amd64.tar.gz
```

### Step 2: Add Go to PATH

```bash
# Add to ~/.bashrc or ~/.zshrc
echo 'export PATH=$HOME/go/bin:$PATH' >> ~/.bashrc
source ~/.bashrc

# Verify
go version
```

### Step 3: Authenticate with Google Cloud

```bash
# Option A: Use gcloud credentials (easiest)
gcloud auth application-default login

# Option B: Or use service account key
# export GOOGLE_APPLICATION_CREDENTIALS="/path/to/key.json"
```

---

## Running a Service Locally

### Game Service

```bash
# 1. Navigate to service directory
cd /home/michal/dev/android/backend/services/game-service

# 2. Set environment variables
export PATH=$HOME/go/bin:$PATH
export GCP_PROJECT=michal-playground-2026

# 3. Download dependencies (first time only)
go mod download

# 4. Run the service
go run main.go
```

You should see:
```
2026/01/23 22:55:20 Starting game-service on port 8080
```

**Service is running on http://localhost:8080**

### User Service

```bash
cd /home/michal/dev/android/backend/services/user-service
export PATH=$HOME/go/bin:$PATH
export GCP_PROJECT=michal-playground-2026
go run main.go
```

---

## Testing Locally

### Browser

```bash
# Open in browser
xdg-open http://localhost:8080

# Or on macOS
open http://localhost:8080
```

### curl

```bash
# Test homepage
curl http://localhost:8080/

# Create a game
curl -X POST http://localhost:8080/games/create -L

# Test specific game (use gameID and token from create response)
curl "http://localhost:8080/games/GAME_ID?token=TOKEN"

# Make a move
curl -X POST "http://localhost:8080/games/GAME_ID/move?token=TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"position": 4}'
```

---

## Development Workflow

### 1. Make Changes

```bash
# Edit any file
nano services/game-service/main.go
nano services/game-service/views/game.html
nano services/game-service/models/game.go
```

### 2. Restart Server

```bash
# In the terminal running the server, press Ctrl+C to stop

# Then run again
go run main.go
```

**Restart takes ~2 seconds** vs 2 minutes for Cloud Run!

### 3. Test Immediately

```bash
# In another terminal
curl http://localhost:8080/
```

### 4. Check Logs

The server logs every request with MVC-style flow:

```
→ homeHandler: GET /                          # Handler invoked
✓ homeHandler: rendered home.html             # Success

→ viewGameHandler: GET /games/abc123          # Handler invoked
  viewGameHandler: player=X, turn=X...        # Context
✓ viewGameHandler: rendered game.html         # Success

✗ viewGameHandler: game not found             # Error
```

**Log symbols:**
- `→` = Handler entry
- `✓` = Success
- `✗` = Error
- ` ` (indent) = Additional context

---

## Verifying Routes

Make sure routes aren't conflicting:

```bash
# Test each route
curl -v http://localhost:8080/                              # homeHandler
curl -v -X POST http://localhost:8080/games/create          # createGameHandler
curl -v "http://localhost:8080/games/test?token=abc"        # viewGameHandler
curl -v -X POST "http://localhost:8080/games/test/move?token=abc" \
  -H "Content-Type: application/json" -d '{"position": 0}'  # makeMoveHandler
```

**Check the logs:** You should see exactly ONE `→` symbol per request.

If you see multiple handlers firing for the same URL, there's a route conflict.

---

## Common Commands

```bash
# Kill running server
pkill -f "go run main.go"
# or just Ctrl+C in the terminal

# Check what's on port 8080
lsof -i :8080

# View logs in real-time
go run main.go  # logs appear in terminal

# Run in background and tail logs
go run main.go > /tmp/server.log 2>&1 &
tail -f /tmp/server.log

# Stop background server
pkill -f "go run"
```

---

## Hot Reload (Optional)

For even faster iteration, use `air` to auto-reload on file changes:

```bash
# Install air
go install github.com/cosmtrek/air@latest

# Run with hot reload
air

# Now just save files and server auto-restarts!
```

---

## Troubleshooting

### "go: command not found"

```bash
export PATH=$HOME/go/bin:$PATH
```

Add to `~/.bashrc` to make permanent.

### "Firestore client: permission denied"

```bash
gcloud auth application-default login
```

### "Template not found"

Make sure you're in the service directory:
```bash
pwd  # Should be: .../services/game-service
ls views/  # Should show: home.html, game.html, layout.html
```

### "Port already in use"

```bash
lsof -ti:8080 | xargs kill
```

### Changes not reflecting

Make sure you restarted the server (Ctrl+C, then `go run main.go` again).

---

## Deploy When Ready

Once you've tested locally:

```bash
# From backend root
cd /home/michal/dev/android/backend

# Deploy to Cloud Run
make deploy-game

# Or manually
cd services/game-service
gcloud run deploy game-service \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars GCP_PROJECT=michal-playground-2026
```

---

## Summary

**Local dev workflow:**
```
Edit code (2s) → Restart server (2s) → Test (instant) → Repeat
```

**vs Cloud Run workflow:**
```
Edit code (2s) → Deploy (2 min) → Test (instant) → Repeat
```

**60x faster iteration!** 🚀
