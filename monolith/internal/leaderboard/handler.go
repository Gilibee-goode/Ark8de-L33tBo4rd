// Package leaderboard — HTTP Handler layer
package leaderboard

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/respond"
)

// LeaderboardHandler handles HTTP requests for leaderboard endpoints.
type LeaderboardHandler struct {
	service *LeaderboardService
}

// NewLeaderboardHandler constructs a LeaderboardHandler.
func NewLeaderboardHandler(service *LeaderboardService) *LeaderboardHandler {
	return &LeaderboardHandler{service: service}
}

// GetLeaderboard handles GET /leaderboard — public.
// Returns all teams in ranked order.
func (h *LeaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	entries, err := h.service.GetLeaderboard(r.Context())
	if err != nil {
		slog.Error("LeaderboardHandler.GetLeaderboard", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load leaderboard")
		return
	}
	respond.JSON(w, http.StatusOK, entries)
}

// GetTeamCard handles GET /leaderboard/teams/:id — public.
// Returns the leaderboard summary card for one team.
func (h *LeaderboardHandler) GetTeamCard(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	entry, err := h.service.GetTeamCard(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, ErrTeamNotFound) {
			respond.Error(w, http.StatusNotFound, "team not found on leaderboard")
			return
		}
		slog.Error("LeaderboardHandler.GetTeamCard", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load team card")
		return
	}
	respond.JSON(w, http.StatusOK, entry)
}
