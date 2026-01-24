package users

import (
	"encoding/json"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/go-chi/chi/v5"
	"github.com/yourusername/my-app-backend/services/game-service/auth"
)

// RegisterRoutes registers user routes
func RegisterRoutes(r chi.Router, client *firestore.Client, authMiddleware func(http.Handler) http.Handler) {
	userService := NewService(client)
	friendService := NewFriendService(client, userService)

	r.Route("/users", func(r chi.Router) {
		r.Use(authMiddleware)

		// GET /users/me - Get current user profile
		r.Get("/me", getMeHandler(userService))

		// PUT /users/me - Update current user profile
		r.Put("/me", updateMeHandler(userService))
	})

	r.Route("/friends", func(r chi.Router) {
		r.Use(authMiddleware)

		// GET /friends - List my friends (with profiles)
		r.Get("/", listFriendsHandler(friendService))

		// GET /friends/{id} - Get friend's profile (only if friends)
		r.Get("/{id}", getFriendProfileHandler(friendService, userService))

		// DELETE /friends/{id} - Remove friend
		r.Delete("/{id}", removeFriendHandler(friendService))

		// POST /friends/invite - Send invite (by email)
		r.Post("/invite", createInviteHandler(friendService, userService))

		// GET /friends/invites - List my pending invites
		r.Get("/invites", listInvitesHandler(friendService))
	})

	// Invite acceptance (can be accessed without auth for link sharing)
	r.Route("/invite", func(r chi.Router) {
		// GET /invite/{token} - View invite details (public)
		r.Get("/{token}", viewInviteHandler(friendService))

		// POST /invite/{token}/accept - Accept invite (requires auth)
		r.With(authMiddleware).Post("/{token}/accept", acceptInviteHandler(friendService))

		// POST /invite/{token}/decline - Decline invite
		r.With(authMiddleware).Post("/{token}/decline", declineInviteHandler(friendService))
	})
}

func getMeHandler(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)
		email := auth.GetUserEmail(ctx)
		name := auth.GetUserName(ctx)

		// Get or create user (ensures user exists on first request)
		user, err := service.GetOrCreate(ctx, userID, email, name)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"data": user,
		})
	}
}

func updateMeHandler(service *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid json",
			})
			return
		}

		// Only allow updating certain fields
		allowed := map[string]bool{
			"name":      true,
			"photo_url": true,
			"bio":       true,
			"location":  true,
		}

		filtered := make(map[string]interface{})
		for k, v := range updates {
			if allowed[k] {
				filtered[k] = v
			}
		}

		user, err := service.Update(ctx, userID, filtered)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "updated",
			"data":    user,
		})
	}
}

// Friend handlers

func listFriendsHandler(friendService *FriendService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		profiles, err := friendService.GetFriendProfiles(ctx, userID)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": profiles})
	}
}

func getFriendProfileHandler(friendService *FriendService, userService *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)
		friendID := chi.URLParam(r, "id")

		// Check if they're friends
		areFriends, err := friendService.AreFriends(ctx, userID, friendID)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		if !areFriends {
			respondJSON(w, http.StatusForbidden, map[string]string{"error": "not friends"})
			return
		}

		// Get friend's profile
		user, err := userService.Get(ctx, friendID)
		if err != nil || user == nil {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": user})
	}
}

func removeFriendHandler(friendService *FriendService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)
		friendID := chi.URLParam(r, "id")

		err := friendService.RemoveFriend(ctx, userID, friendID)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "friend removed"})
	}
}

func createInviteHandler(friendService *FriendService, userService *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if req.Email == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "email required"})
			return
		}

		// Get sender's name
		sender, _ := userService.Get(ctx, userID)
		senderName := userID
		if sender != nil && sender.Name != "" {
			senderName = sender.Name
		}

		invite, err := friendService.CreateInvite(ctx, userID, senderName, req.Email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message":    "invite created",
			"data":       invite,
			"invite_url": "/invite/" + invite.Token,
		})
	}
}

func listInvitesHandler(friendService *FriendService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)

		invites, err := friendService.GetPendingInvites(ctx, userID)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{"data": invites})
	}
}

func viewInviteHandler(friendService *FriendService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := chi.URLParam(r, "token")

		invite, err := friendService.GetInviteByToken(ctx, token)
		if err != nil {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "invite not found"})
			return
		}

		// Only return safe info
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"sender_name": invite.SenderName,
				"status":      invite.Status,
				"created_at":  invite.CreatedAt,
			},
		})
	}
}

func acceptInviteHandler(friendService *FriendService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)
		token := chi.URLParam(r, "token")

		friendship, err := friendService.AcceptInvite(ctx, token, userID)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "invite not found" {
				status = http.StatusNotFound
			} else if err.Error() == "cannot accept your own invite" {
				status = http.StatusBadRequest
			}
			respondJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"message": "you are now friends!",
			"data":    friendship,
		})
	}
}

func declineInviteHandler(friendService *FriendService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := auth.GetUserID(ctx)
		token := chi.URLParam(r, "token")

		err := friendService.DeclineInvite(ctx, token, userID)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{"message": "invite declined"})
	}
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
