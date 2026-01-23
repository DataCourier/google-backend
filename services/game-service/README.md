# Game Service - Tic-Tac-Toe

Multiplayer tic-tac-toe with URL-based game rooms. No authentication required.

## Features

- ✨ Create game with unique UUID
- 🔗 Share link with opponent
- 🎮 Turn-based gameplay
- ✅ Move validation
- 🏆 Win detection
- 🔄 Auto-refresh when waiting for opponent

## How It Works

1. Player visits homepage
2. Creates new game → Gets player X URL
3. Shares opponent URL with friend
4. Players make moves via their unique tokens
5. Game detects winner or draw

## Database

Uses Firestore to persist game state:

```go
type Game struct {
    ID           string    // Public game ID (UUID)
    PlayerXToken string    // Secret token for player X
    PlayerOToken string    // Secret token for player O
    Board        [9]string // Board state
    CurrentTurn  string    // "X" or "O"
    Winner       string    // "", "X", "O", or "draw"
}
```

## Endpoints

- `GET /` - Homepage
- `POST /games/create` - Create new game, redirect to player X URL
- `GET /games/{gameId}?token={token}` - View game board
- `POST /games/{gameId}/move?token={token}` - Make move (JSON API)

## Local Development

```bash
# Set project ID
export GCP_PROJECT=your-project-id

# Run locally
go run main.go

# Visit http://localhost:8080
```

## Deploy

```bash
gcloud run deploy game-service \
  --source . \
  --region us-central1 \
  --allow-unauthenticated \
  --set-env-vars GCP_PROJECT=your-project-id
```

## What It Demonstrates

- ✅ Database writes (create game)
- ✅ Database reads (get game state)
- ✅ Database updates (make move)
- ✅ Validation (turn-based, position checking)
- ✅ Game logic (win detection)
- ✅ Token-based access control
- ✅ HTML templates with Go stdlib
- ✅ REST API (JSON responses)

Perfect learning example for the portable backend architecture!
