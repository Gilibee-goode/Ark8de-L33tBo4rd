// Package team — HTTP Handler layer
//
// Translates HTTP requests into service calls for all team-related operations.
package team

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

// TeamHandler handles HTTP requests for team endpoints.
type TeamHandler struct {
	service *TeamService
}

// NewTeamHandler constructs a TeamHandler.
func NewTeamHandler(service *TeamService) *TeamHandler {
	return &TeamHandler{service: service}
}

// ListTeams handles GET /teams — public.
func (h *TeamHandler) ListTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := h.service.ListTeams(r.Context())
	if err != nil {
		slog.Error("TeamHandler.ListTeams", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load teams")
		return
	}
	respond.JSON(w, http.StatusOK, teams)
}

// GetTeam handles GET /teams/:id — public.
func (h *TeamHandler) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	detail, err := h.service.GetTeamDetail(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, ErrTeamNotFound) {
			respond.Error(w, http.StatusNotFound, "team not found")
			return
		}
		slog.Error("TeamHandler.GetTeam", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load team")
		return
	}
	respond.JSON(w, http.StatusOK, detail)
}

// CreateTeam handles POST /teams — any authenticated player.
func (h *TeamHandler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	ownerID := middleware.PlayerIDFromContext(r.Context())
	var req CreateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"name\": \"...\", \"tag\": \"...\"}")
		return
	}
	team, err := h.service.CreateTeam(r.Context(), ownerID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTeamName), errors.Is(err, ErrInvalidTag):
			respond.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrTeamNameTaken), errors.Is(err, ErrTagTaken):
			respond.Error(w, http.StatusConflict, err.Error())
		default:
			slog.Error("TeamHandler.CreateTeam", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to create team")
		}
		return
	}
	respond.JSON(w, http.StatusCreated, team)
}

// UpdateTeam handles PUT /teams/:id — team_owner or admin.
func (h *TeamHandler) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	requesterRole := middleware.RoleFromContext(r.Context())
	teamID := chi.URLParam(r, "id")

	var req UpdateTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.service.UpdateTeam(r.Context(), requesterID, requesterRole, teamID, req); err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrForbidden):
			respond.Error(w, http.StatusForbidden, err.Error())
		case errors.Is(err, ErrInvalidTeamName), errors.Is(err, ErrInvalidTag):
			respond.Error(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrTeamNameTaken), errors.Is(err, ErrTagTaken):
			respond.Error(w, http.StatusConflict, err.Error())
		default:
			slog.Error("TeamHandler.UpdateTeam", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to update team")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "team updated"})
}

// DeleteTeam handles DELETE /teams/:id — team_owner or admin.
func (h *TeamHandler) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	requesterRole := middleware.RoleFromContext(r.Context())
	teamID := chi.URLParam(r, "id")

	if err := h.service.DeleteTeam(r.Context(), requesterID, requesterRole, teamID); err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrForbidden):
			respond.Error(w, http.StatusForbidden, err.Error())
		default:
			slog.Error("TeamHandler.DeleteTeam", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to delete team")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "team deleted"})
}

// ToggleLock handles PUT /teams/:id/lock — team_owner or admin.
func (h *TeamHandler) ToggleLock(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	requesterRole := middleware.RoleFromContext(r.Context())
	teamID := chi.URLParam(r, "id")

	locked, err := h.service.ToggleLock(r.Context(), requesterID, requesterRole, teamID)
	if err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrForbidden):
			respond.Error(w, http.StatusForbidden, err.Error())
		default:
			slog.Error("TeamHandler.ToggleLock", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to toggle lock")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]bool{"is_locked_in": locked})
}

// RemoveMember handles DELETE /teams/:id/members/:pid — team_owner or admin.
func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	requesterRole := middleware.RoleFromContext(r.Context())
	teamID := chi.URLParam(r, "id")
	playerID := chi.URLParam(r, "pid")

	if err := h.service.RemoveMember(r.Context(), requesterID, requesterRole, teamID, playerID); err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrForbidden):
			respond.Error(w, http.StatusForbidden, err.Error())
		case errors.Is(err, ErrCannotRemoveOwner):
			respond.Error(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("TeamHandler.RemoveMember", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to remove member")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "member removed"})
}

// SendJoinRequest handles POST /teams/:id/join-requests — any authenticated player.
func (h *TeamHandler) SendJoinRequest(w http.ResponseWriter, r *http.Request) {
	playerID := middleware.PlayerIDFromContext(r.Context())
	teamID := chi.URLParam(r, "id")

	req, err := h.service.SendJoinRequest(r.Context(), playerID, teamID)
	if err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrTeamLocked), errors.Is(err, ErrAlreadyInTeam), errors.Is(err, ErrAlreadyRequested):
			respond.Error(w, http.StatusConflict, err.Error())
		default:
			slog.Error("TeamHandler.SendJoinRequest", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to send join request")
		}
		return
	}
	respond.JSON(w, http.StatusCreated, req)
}

// GetJoinRequests handles GET /teams/:id/join-requests — team_owner or admin.
func (h *TeamHandler) GetJoinRequests(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	requesterRole := middleware.RoleFromContext(r.Context())
	teamID := chi.URLParam(r, "id")

	requests, err := h.service.GetJoinRequests(r.Context(), requesterID, requesterRole, teamID)
	if err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrForbidden):
			respond.Error(w, http.StatusForbidden, err.Error())
		default:
			slog.Error("TeamHandler.GetJoinRequests", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to load join requests")
		}
		return
	}
	respond.JSON(w, http.StatusOK, requests)
}

// ResolveJoinRequest handles PUT /teams/:id/join-requests/:rid — team_owner or admin.
func (h *TeamHandler) ResolveJoinRequest(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	requesterRole := middleware.RoleFromContext(r.Context())
	teamID := chi.URLParam(r, "id")
	requestID := chi.URLParam(r, "rid")

	var body ResolveJoinRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"action\": \"accept|reject\"}")
		return
	}

	if err := h.service.ResolveJoinRequest(r.Context(), requesterID, requesterRole, teamID, requestID, body.Action); err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound), errors.Is(err, ErrJoinRequestNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrForbidden):
			respond.Error(w, http.StatusForbidden, err.Error())
		case errors.Is(err, ErrInvalidAction), errors.Is(err, ErrRequestNotPending):
			respond.Error(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("TeamHandler.ResolveJoinRequest", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to resolve join request")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "join request " + body.Action + "ed"})
}

// AddArkadePoints handles PUT /teams/:id/points — moderator or admin.
func (h *TeamHandler) AddArkadePoints(w http.ResponseWriter, r *http.Request) {
	requesterID := middleware.PlayerIDFromContext(r.Context())
	teamID := chi.URLParam(r, "id")

	var req AddArkadePointsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body — expected {\"delta\": 10, \"reason\": \"...\"}")
		return
	}

	if err := h.service.AddArkadePoints(r.Context(), requesterID, teamID, req); err != nil {
		switch {
		case errors.Is(err, ErrTeamNotFound):
			respond.Error(w, http.StatusNotFound, err.Error())
		case errors.Is(err, ErrInvalidDelta):
			respond.Error(w, http.StatusBadRequest, err.Error())
		default:
			slog.Error("TeamHandler.AddArkadePoints", "error", err)
			respond.Error(w, http.StatusInternalServerError, "failed to update Arkade points")
		}
		return
	}
	respond.JSON(w, http.StatusOK, map[string]string{"status": "Arkade points updated"})
}

// GetArkadePointHistory handles GET /teams/:id/points/history — moderator or admin.
func (h *TeamHandler) GetArkadePointHistory(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")

	logs, err := h.service.GetArkadePointHistory(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, ErrTeamNotFound) {
			respond.Error(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("TeamHandler.GetArkadePointHistory", "error", err)
		respond.Error(w, http.StatusInternalServerError, "failed to load point history")
		return
	}
	respond.JSON(w, http.StatusOK, logs)
}

// UploadLogo handles PUT /teams/:id/logo — not implemented in Phase 1.
func (h *TeamHandler) UploadLogo(w http.ResponseWriter, r *http.Request) {
	respond.Error(w, http.StatusNotImplemented, "logo upload will be implemented in Phase 2 with object storage")
}
