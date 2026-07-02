// Package player — HTTP Handler layer
//
// Translates HTTP requests to service calls and writes JSON responses.
// No SQL, no business rules — only: parse → call → respond.
package player

import (
	// --- Standard library ---
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	// --- Third-party ---
	"github.com/go-chi/chi/v5"

	// --- Internal ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/middleware"
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/respond"
)

// PlayerHandler handles HTTP requests for player-related endpoints.
type PlayerHandler struct {
	service *PlayerService
}

// NewPlayerHandler constructs a PlayerHandler.
func NewPlayerHandler(service *PlayerService) *PlayerHandler {
	return &PlayerHandler{service: service}
}

// GetPublicProfile handles GET /players/:id
// Returns the public profile of any player. No auth required.
func (h *PlayerHandler) GetPublicProfile(w http.ResponseWriter, r *http.Request) {
	// chi.URLParam extracts a named path parameter from the URL.
	// For a route registered as "/players/{id}", chi.URLParam(r, "id") returns the :id value.
	playerID := chi.URLParam(r, "id")

	profile, err := h.service.GetPublicProfile(r.Context(), playerID)
	if err != nil {
		if errors.Is(err, ErrPlayerNotFound) {
			respond.Error(w, http.StatusNotFound, "player not found")
			return
		}
		slog.Error("PlayerHandler.GetPublicProfile: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load player profile")
		return
	}
	respond.JSON(w, http.StatusOK, profile)
}

// SetClass handles PUT /players/me/class
// Sets the authenticated player's class role.
func (h *PlayerHandler) SetClass(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	var req SetClassRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"class_role\": \"tank|dps|healer|support\"}")
		return
	}

	if err := h.service.SetClass(r.Context(), playerID, req.ClassRole); err != nil {
		if errors.Is(err, ErrInvalidClassRole) {
			respond.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("PlayerHandler.SetClass: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to update class")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"class_role": req.ClassRole})
}

// GetStats handles GET /players/me/stats
// Returns the authenticated player's computed combat statistics.
func (h *PlayerHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	stats, err := h.service.GetStats(r.Context(), playerID)
	if err != nil {
		slog.Error("PlayerHandler.GetStats: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to compute stats")
		return
	}
	respond.JSON(w, http.StatusOK, stats)
}

// GetSkills handles GET /players/me/skills
// Returns the player's allocated skills and remaining skill point budget.
func (h *PlayerHandler) GetSkills(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	skills, err := h.service.GetSkills(r.Context(), playerID)
	if err != nil {
		slog.Error("PlayerHandler.GetSkills: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load skills")
		return
	}
	respond.JSON(w, http.StatusOK, skills)
}

// SetSkills handles PUT /players/me/skills
// Replaces the player's entire skill allocation.
func (h *PlayerHandler) SetSkills(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	var req SetSkillsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"skill_ids\": [\"uuid\", ...]}")
		return
	}

	if err := h.service.SetSkills(r.Context(), playerID, req); err != nil {
		switch {
		case errors.Is(err, ErrNoClassSet),
			errors.Is(err, ErrSkillNotFound),
			errors.Is(err, ErrSkillWrongClass),
			errors.Is(err, ErrTierAboveLevel),
			errors.Is(err, ErrOneSkillPerTier):
			respond.Error(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("PlayerHandler.SetSkills: unexpected error", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to update skills")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "skills updated"})
}

// GetGear handles GET /players/me/gear
// Returns the player's current gear and the team's gear pool status.
func (h *PlayerHandler) GetGear(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	gear, err := h.service.GetGear(r.Context(), playerID)
	if err != nil {
		slog.Error("PlayerHandler.GetGear: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load gear")
		return
	}
	respond.JSON(w, http.StatusOK, gear)
}

// SetGear handles PUT /players/me/gear
// Replaces the player's entire gear selection.
func (h *PlayerHandler) SetGear(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	var req SetGearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"gear_type_ids\": [\"uuid\", ...]}")
		return
	}

	if err := h.service.SetGear(r.Context(), playerID, req); err != nil {
		if errors.Is(err, ErrGearNotFound) || errors.Is(err, ErrGearClassRestricted) {
			respond.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("PlayerHandler.SetGear: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to update gear")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "gear updated"})
}

// SetLevel handles PUT /players/:id/level — moderator/admin only.
// Sets a player's level (1–3); leveling down prunes now-illegal skills.
func (h *PlayerHandler) SetLevel(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "id")

	var req SetLevelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"level\": 2}")
		return
	}

	if err := h.service.SetLevel(r.Context(), playerID, req); err != nil {
		switch {
		case errors.Is(err, ErrInvalidLevel):
			respond.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrPlayerNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		default:
			slog.Error("PlayerHandler.SetLevel: unexpected error", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to set level")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "level updated"})
}

// GetKredits handles GET /players/me/kredits
// Returns the player's Kredit balance and transaction history.
func (h *PlayerHandler) GetKredits(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())

	kredits, err := h.service.GetKredits(r.Context(), playerID)
	if err != nil {
		slog.Error("PlayerHandler.GetKredits: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load Kredits")
		return
	}
	respond.JSON(w, http.StatusOK, kredits)
}

// GrantKredits handles POST /players/:id/kredits — moderator/admin only.
// Grants Kredits to a target player without a sender (moderation action).
func (h *PlayerHandler) GrantKredits(w http.ResponseWriter, r *http.Request) {
	moderatorID := middleware.PlayerIDFromContext(r.Context())
	toPlayerID := chi.URLParam(r, "id")

	var req GrantKreditsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"amount\": 100, \"note\": \"...\"}")
		return
	}

	if err := h.service.GrantKredits(r.Context(), toPlayerID, moderatorID, req); err != nil {
		switch {
		case errors.Is(err, ErrInvalidAmount):
			respond.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrPlayerNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		default:
			slog.Error("PlayerHandler.GrantKredits: unexpected error", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to grant Kredits")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "Kredits granted"})
}

// TransferKredits handles POST /players/me/kredits/transfer
// Moves Kredits from the authenticated player to another player.
func (h *PlayerHandler) TransferKredits(w http.ResponseWriter, r *http.Request) {
	fromPlayerID := middleware.PlayerIDFromContext(r.Context())

	var req TransferKreditsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"to_player_id\": \"uuid\", \"amount\": 50, \"note\": \"...\"}")
		return
	}

	if err := h.service.TransferKredits(r.Context(), fromPlayerID, req); err != nil {
		switch {
		case errors.Is(err, ErrInvalidAmount),
			errors.Is(err, ErrCannotTransferToSelf),
			errors.Is(err, ErrInsufficientKredits):
			respond.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrPlayerNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		default:
			slog.Error("PlayerHandler.TransferKredits: unexpected error", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to transfer Kredits")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "Kredits transferred"})
}

// ListSkills handles GET /skills?class_role=tank
// Public endpoint — returns all skills, optionally filtered by class role.
func (h *PlayerHandler) ListSkills(w http.ResponseWriter, r *http.Request) {
	// r.URL.Query().Get reads a query parameter from the URL.
	// For /skills?class_role=tank, this returns "tank".
	classRole := r.URL.Query().Get("class_role")

	skills, err := h.service.GetAvailableSkills(r.Context(), classRole)
	if err != nil {
		if errors.Is(err, ErrInvalidClassRole) {
			respond.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		slog.Error("PlayerHandler.ListSkills: unexpected error", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load skills")
		return
	}
	respond.JSON(w, http.StatusOK, skills)
}
