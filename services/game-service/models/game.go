package models

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/google/uuid"
)

const GamesCollection = "games"

type Game struct {
	ID           string    `firestore:"id" json:"id"`
	PlayerXToken string    `firestore:"playerXToken" json:"-"` // Secret, don't expose in JSON
	PlayerOToken string    `firestore:"playerOToken" json:"-"` // Secret, don't expose in JSON
	Board        [9]string `firestore:"board" json:"board"`    // ["", "X", "O", "", ...]
	CurrentTurn  string    `firestore:"currentTurn" json:"currentTurn"` // "X" or "O"
	Winner       string    `firestore:"winner" json:"winner"`   // "", "X", "O", "draw"
	CreatedAt    time.Time `firestore:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time `firestore:"updatedAt" json:"updatedAt"`
}

// CreateGame creates a new tic-tac-toe game
func CreateGame(ctx context.Context, client *firestore.Client) (*Game, error) {
	game := &Game{
		ID:           uuid.New().String(),
		PlayerXToken: uuid.New().String(),
		PlayerOToken: uuid.New().String(),
		Board:        [9]string{"", "", "", "", "", "", "", "", ""},
		CurrentTurn:  "X",
		Winner:       "",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err := client.Collection(GamesCollection).Doc(game.ID).Set(ctx, game)
	if err != nil {
		return nil, err
	}

	return game, nil
}

// GetGame fetches a game by ID
func GetGame(ctx context.Context, client *firestore.Client, id string) (*Game, error) {
	doc, err := client.Collection(GamesCollection).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}

	var game Game
	if err := doc.DataTo(&game); err != nil {
		return nil, err
	}

	return &game, nil
}

// ValidateToken checks if the token belongs to this game
func (g *Game) ValidateToken(token string) (string, error) {
	if token == g.PlayerXToken {
		return "X", nil
	}
	if token == g.PlayerOToken {
		return "O", nil
	}
	return "", errors.New("invalid token")
}

// MakeMove attempts to make a move on the board
func (g *Game) MakeMove(ctx context.Context, client *firestore.Client, player string, position int) error {
	// Validation
	if g.Winner != "" {
		return errors.New("game is already finished")
	}

	if player != g.CurrentTurn {
		return errors.New("not your turn")
	}

	if position < 0 || position > 8 {
		return errors.New("invalid position (must be 0-8)")
	}

	if g.Board[position] != "" {
		return errors.New("position already taken")
	}

	// Make the move
	g.Board[position] = player
	g.UpdatedAt = time.Now()

	// Check for winner
	g.Winner = g.CheckWinner()

	// Switch turn if game not over
	if g.Winner == "" {
		if g.CurrentTurn == "X" {
			g.CurrentTurn = "O"
		} else {
			g.CurrentTurn = "X"
		}
	}

	// Save to database
	_, err := client.Collection(GamesCollection).Doc(g.ID).Set(ctx, g)
	return err
}

// CheckWinner determines if there's a winner or draw
func (g *Game) CheckWinner() string {
	// Winning combinations
	lines := [][]int{
		{0, 1, 2}, {3, 4, 5}, {6, 7, 8}, // Rows
		{0, 3, 6}, {1, 4, 7}, {2, 5, 8}, // Columns
		{0, 4, 8}, {2, 4, 6},            // Diagonals
	}

	for _, line := range lines {
		if g.Board[line[0]] != "" &&
			g.Board[line[0]] == g.Board[line[1]] &&
			g.Board[line[1]] == g.Board[line[2]] {
			return g.Board[line[0]] // "X" or "O"
		}
	}

	// Check for draw (board full)
	full := true
	for _, cell := range g.Board {
		if cell == "" {
			full = false
			break
		}
	}

	if full {
		return "draw"
	}

	return "" // Game ongoing
}
