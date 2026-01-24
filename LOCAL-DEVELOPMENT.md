# Local Development Guide

Test changes locally before deploying to Cloud Run (way faster than 2-minute deploys!).

---

## Quick Start

**Using Makefile (easiest):**

```bash
# Terminal 1: Start Firestore Emulator
make emulator

# Terminal 2: Run game-service
make dev-game

# Terminal 3: Test
curl http://localhost:8080/
```

**Manual (if you prefer):**

```bash
# Terminal 1: Start Firestore Emulator
gcloud emulators firestore start

# Terminal 2: Run the service
cd services/game-service
export PATH=$HOME/go/bin:$PATH
export GCP_PROJECT=michal-playground-2026
export FIRESTORE_EMULATOR_HOST=localhost:8080
go run .

# Terminal 3: Test
curl http://localhost:8080/
```

**Why use the emulator?**
- ⚡ Instant startup (~100ms vs 2s network roundtrip)
- 🔒 Isolated test data (won't pollute production)
- ✈️ Works offline
- 🚨 Server will error if emulator not running (prevents accidental prod writes)

**Template Hot Reload:**
- Edit HTML/CSS in `views/*.html` → Just refresh browser (instant!)
- No server restart needed for template changes
- Only restart when changing Go code

**⚠️ What if I forget to start the emulator?**

The server will fail immediately with a helpful error:

```
🚨 ERROR: Firestore emulator not detected!

In local development, you MUST use the Firestore emulator to avoid writing to production.

To fix:
  1. Terminal 1: gcloud emulators firestore start
  2. Terminal 2: export FIRESTORE_EMULATOR_HOST=localhost:8080
  3. Then run: go run .
```

This prevents you from accidentally writing test data to production Firestore!

---

## Prerequisites

### 1. Install Firestore Emulator

```bash
# Install the emulator component
gcloud components install cloud-firestore-emulator

# Verify it's installed
gcloud emulators firestore --help
```

**Start the emulator** (leave this running in a dedicated terminal):

```bash
gcloud emulators firestore start
```

You should see:
```
[firestore] API endpoint: http://localhost:8080
```

Keep this terminal open! The emulator needs to stay running.

### 2. Install Go

```bash
# Check if Go is installed
go version

# If not installed, install Go 1.23+
wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.23.0.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
export PATH=$PATH:/usr/local/go/bin

# Verify
go version
```

### 2. Set Up Firebase Credentials

**Option A: Use your existing gcloud credentials** (easiest)
```bash
# gcloud will automatically use your credentials
gcloud auth application-default login
```

**Option B: Service account key** (if needed)
```bash
# Download service account key from Firebase Console
# https://console.firebase.google.com/project/YOUR_PROJECT/settings/serviceaccounts

# Set environment variable
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/serviceAccountKey.json"
```

---

## Running Services Locally

### Game Service

```bash
cd services/game-service

# Set project ID
export GCP_PROJECT=michal-playground-2026

# Install dependencies
go mod download

# Run
go run main.go
```

Visit: http://localhost:8080

### User Service

```bash
cd services/user-service

# Set project ID
export GCP_PROJECT=michal-playground-2026

# Run
go run main.go
```

Visit: http://localhost:8080

---

## Local Development Workflow

### 1. Make Changes

Edit files in `services/game-service/`:
```bash
nano main.go
nano views/game.html
nano models/game.go
```

### 2. Test Locally

```bash
# Kill previous process (Ctrl+C or)
pkill -f "go run"

# Run again
go run main.go
```

Changes take ~1-2 seconds to restart (vs 2 minutes for Cloud Run!).

### 3. Test in Browser

```bash
# Open in browser
xdg-open http://localhost:8080

# Or use curl
curl http://localhost:8080/
```

### 4. Deploy When Ready

```bash
# From backend root
make deploy-game

# Or manually
gcloud run deploy game-service --source . --region us-central1 --allow-unauthenticated
```

---

## Hot Reload (Optional)

For even faster iteration, use `air` for auto-reload on file changes:

```bash
# Install air
go install github.com/cosmtrek/air@latest

# Create .air.toml config
cat > .air.toml << 'EOF'
root = "."
testdata_dir = "testdata"
tmp_dir = "tmp"

[build]
  args_bin = []
  bin = "./tmp/main"
  cmd = "go build -o ./tmp/main ."
  delay = 1000
  exclude_dir = ["assets", "tmp", "vendor", "testdata"]
  exclude_file = []
  exclude_regex = ["_test.go"]
  exclude_unchanged = false
  follow_symlink = false
  full_bin = ""
  include_dir = []
  include_ext = ["go", "tpl", "tmpl", "html"]
  include_file = []
  kill_delay = "0s"
  log = "build-errors.log"
  poll = false
  poll_interval = 0
  rerun = false
  rerun_delay = 500
  send_interrupt = false
  stop_on_error = false

[color]
  app = ""
  build = "yellow"
  main = "magenta"
  runner = "green"
  watcher = "cyan"

[log]
  main_only = false
  time = false

[misc]
  clean_on_exit = false

[screen]
  clear_on_rebuild = false
  keep_scroll = true
EOF

# Run with hot reload
air
```

Now edit files and they auto-reload!

---

## Understanding the Logs

The service now logs each request with MVC-style flow:

```
→ homeHandler: GET /                          # Handler entry
✓ homeHandler: rendered home.html             # Success

→ createGameHandler: POST /games/create       # Handler entry
✓ createGameHandler: created game abc123...   # Success + redirect

→ viewGameHandler: GET /games/abc123          # Handler entry
  viewGameHandler: player=X, turn=X...        # Context info
  viewGameHandler: rendering template...       # About to render
✓ viewGameHandler: rendered game.html         # Success

✗ viewGameHandler: game not found             # Error
```

**Log symbols:**
- `→` - Handler invoked
- `✓` - Successful completion
- `✗` - Error occurred
- ` ` (indent) - Additional context

This helps you see:
- Which handler is executing
- If multiple handlers fire for one URL
- Where failures occur
- Template rendering flow

### Debugging

### Print Debugging

```go
// Add to handlers
log.Printf("DEBUG: gameID=%s, token=%s", gameID, token)
log.Printf("DEBUG: game data: %+v", game)
```

Logs appear in terminal immediately.

### Check Firestore Data

```bash
# List all games
gcloud firestore databases documents list --collection=games

# Get specific game
gcloud firestore databases documents get games/GAME_ID
```

### Test API Endpoints

```bash
# Create game
curl -X POST http://localhost:8080/games/create -L

# View game
curl "http://localhost:8080/games/GAME_ID?token=TOKEN"

# Make move
curl -X POST "http://localhost:8080/games/GAME_ID/move?token=TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"position": 4}'
```

---

## Common Issues

### "Firestore client: permission denied"

Make sure you're authenticated:
```bash
gcloud auth application-default login
```

### "Template not found"

Make sure you're running from the service directory:
```bash
cd services/game-service  # Important!
go run main.go
```

Templates are loaded from `views/` relative to current directory.

### Port already in use

Kill the previous process:
```bash
lsof -ti:8080 | xargs kill
# or
pkill -f "go run"
```

### Changes not reflecting

Make sure you restarted the server (Ctrl+C, then run again).

---

## Tips

### Use tmux/screen for multiple services

```bash
# Terminal 1
cd services/game-service && go run main.go

# Terminal 2
cd services/user-service && go run main.go

# Terminal 3
curl http://localhost:8080  # test game-service
curl http://localhost:8081  # test user-service
```

### Verify Routes Are Working

```bash
# Test each route and check logs
curl -v http://localhost:8080/                              # Should hit homeHandler
curl -v -X POST http://localhost:8080/games/create          # Should hit createGameHandler
curl -v "http://localhost:8080/games/test?token=abc"        # Should hit viewGameHandler
curl -v -X POST "http://localhost:8080/games/test/move?token=abc" \
  -H "Content-Type: application/json" \
  -d '{"position": 0}'                                       # Should hit makeMoveHandler
```

Check the logs - you should see exactly one `→` per request (one handler fires).

If you see multiple handlers firing for one URL, there's a route conflict.

### Quick test script

```bash
#!/bin/bash
# test-local.sh

echo "Creating game..."
LOCATION=$(curl -sI -X POST http://localhost:8080/games/create | grep location: | cut -d' ' -f2 | tr -d '\r')
FULL_URL="http://localhost:8080$LOCATION"

echo "Game URL: $FULL_URL"
echo "Opening in browser..."
xdg-open "$FULL_URL"
```

---

## Next Steps

- Try local dev now (way faster!)
- Deploy when ready with `make deploy-game`
- Local dev = instant feedback, Cloud Run = production testing
