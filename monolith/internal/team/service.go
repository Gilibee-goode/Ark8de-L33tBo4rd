// Package team — Business Logic / Service layer
//
// Business rules enforced here:
//   - Team name (3-50 chars) and tag (2-10 chars, uppercase) validation
//   - A player cannot create a team if they already own one
//   - A player cannot join a team if already a member of any team
//   - Only the team's own owner (or an admin) may edit/delete it
//   - Only the team's owner (or a team_owner+) may view join requests
//   - Arkade point delta cannot be 0
package team

import (
	// --- Standard library ---
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	// --- Third-party ---
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrTeamNotFound        = errors.New("team not found")
	ErrJoinRequestNotFound = errors.New("join request not found")
	ErrInvalidTeamName     = errors.New("team name must be 3–50 characters")
	ErrInvalidTag          = errors.New("team tag must be 2–10 characters (letters and digits only)")
	ErrTeamNameTaken       = errors.New("a team with that name already exists")
	ErrTagTaken            = errors.New("a team with that tag already exists")
	ErrForbidden           = errors.New("you do not have permission to perform this action")
	ErrAlreadyInTeam       = errors.New("you are already a member of a team")
	ErrAlreadyRequested    = errors.New("you already have a pending join request for this team")
	ErrTeamLocked          = errors.New("this team is locked — no changes are allowed")
	ErrNotMember           = errors.New("that player is not a member of this team")
	ErrInvalidDelta        = errors.New("point delta cannot be 0")
	ErrCannotRemoveOwner   = errors.New("cannot remove the team owner — delete the team instead")
	ErrInvalidAction       = errors.New("action must be 'accept' or 'reject'")
	ErrRequestNotPending   = errors.New("this join request is no longer pending")
)

// TeamService contains all business logic for team operations.
type TeamService struct {
	repo *TeamRepository
}

// NewTeamService constructs a TeamService.
func NewTeamService(repo *TeamRepository) *TeamService {
	return &TeamService{repo: repo}
}

// ListTeams returns all teams sorted by Arkade points.
func (s *TeamService) ListTeams(ctx context.Context) ([]*Team, error) {
	teams, err := s.repo.ListTeams(ctx)
	if err != nil {
		return nil, fmt.Errorf("TeamService.ListTeams: %w", err)
	}
	if teams == nil {
		teams = []*Team{}
	}
	return teams, nil
}

// GetTeamDetail returns a team with its full roster and gear pool status.
func (s *TeamService) GetTeamDetail(ctx context.Context, teamID string) (*TeamDetailResponse, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("TeamService.GetTeamDetail: %w", err)
	}

	members, err := s.repo.GetMembers(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.GetTeamDetail: members: %w", err)
	}

	gearUsed, err := s.repo.GetTeamGearUsed(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.GetTeamDetail: gear: %w", err)
	}

	ownerUsername, err := s.repo.GetOwnerUsername(ctx, team.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.GetTeamDetail: owner: %w", err)
	}

	if members == nil {
		members = []TeamMember{}
	}

	return &TeamDetailResponse{
		Team:             team,
		Members:          members,
		GearPointsUsed:   gearUsed,
		GearPoolNegative: gearUsed > team.GearPointsTotal,
		OwnerUsername:    ownerUsername,
	}, nil
}

// CreateTeam creates a new team with the given player as owner.
func (s *TeamService) CreateTeam(ctx context.Context, ownerID string, req CreateTeamRequest) (*Team, error) {
	name := strings.TrimSpace(req.Name)
	tag := strings.ToUpper(strings.TrimSpace(req.Tag))

	if len(name) < 3 || len(name) > 50 {
		return nil, ErrInvalidTeamName
	}
	if err := validateTag(tag); err != nil {
		return nil, err
	}

	team, err := s.repo.CreateTeam(ctx, name, tag, ownerID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "name") {
				return nil, ErrTeamNameTaken
			}
			if strings.Contains(pgErr.ConstraintName, "tag") {
				return nil, ErrTagTaken
			}
		}
		return nil, fmt.Errorf("TeamService.CreateTeam: %w", err)
	}
	return team, nil
}

// UpdateTeam changes a team's name and tag. Only the owner or admin may do this.
func (s *TeamService) UpdateTeam(ctx context.Context, requesterID, requesterRole, teamID string, req UpdateTeamRequest) error {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTeamNotFound
		}
		return fmt.Errorf("TeamService.UpdateTeam: %w", err)
	}

	if requesterRole != "admin" && team.OwnerID != requesterID {
		return ErrForbidden
	}

	name := strings.TrimSpace(req.Name)
	tag := strings.ToUpper(strings.TrimSpace(req.Tag))
	if len(name) < 3 || len(name) > 50 {
		return ErrInvalidTeamName
	}
	if err := validateTag(tag); err != nil {
		return err
	}

	if err := s.repo.UpdateTeam(ctx, teamID, name, tag); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "name") {
				return ErrTeamNameTaken
			}
			if strings.Contains(pgErr.ConstraintName, "tag") {
				return ErrTagTaken
			}
		}
		return fmt.Errorf("TeamService.UpdateTeam: %w", err)
	}
	return nil
}

// DeleteTeam removes a team. Only the owner or admin may do this.
func (s *TeamService) DeleteTeam(ctx context.Context, requesterID, requesterRole, teamID string) error {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTeamNotFound
		}
		return fmt.Errorf("TeamService.DeleteTeam: %w", err)
	}

	if requesterRole != "admin" && team.OwnerID != requesterID {
		return ErrForbidden
	}

	return s.repo.DeleteTeam(ctx, teamID, team.OwnerID)
}

// ToggleLock flips the team's lock state. Only the owner or admin may do this.
func (s *TeamService) ToggleLock(ctx context.Context, requesterID, requesterRole, teamID string) (bool, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrTeamNotFound
		}
		return false, fmt.Errorf("TeamService.ToggleLock: %w", err)
	}
	if requesterRole != "admin" && team.OwnerID != requesterID {
		return false, ErrForbidden
	}

	newState, err := s.repo.ToggleLock(ctx, teamID)
	if err != nil {
		return false, fmt.Errorf("TeamService.ToggleLock: %w", err)
	}
	return newState, nil
}

// RemoveMember removes a player from a team. Only the owner or admin may do this.
func (s *TeamService) RemoveMember(ctx context.Context, requesterID, requesterRole, teamID, playerID string) error {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTeamNotFound
		}
		return fmt.Errorf("TeamService.RemoveMember: %w", err)
	}

	if requesterRole != "admin" && team.OwnerID != requesterID {
		return ErrForbidden
	}
	if team.OwnerID == playerID {
		return ErrCannotRemoveOwner
	}

	return s.repo.RemoveMember(ctx, teamID, playerID)
}

// SendJoinRequest creates a pending join request from a player to a team.
func (s *TeamService) SendJoinRequest(ctx context.Context, playerID, teamID string) (*JoinRequest, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("TeamService.SendJoinRequest: %w", err)
	}
	if team.IsLockedIn {
		return nil, ErrTeamLocked
	}

	alreadyMember, err := s.repo.IsMember(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.SendJoinRequest: %w", err)
	}
	if alreadyMember {
		return nil, ErrAlreadyInTeam
	}

	alreadyRequested, err := s.repo.HasPendingRequest(ctx, teamID, playerID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.SendJoinRequest: %w", err)
	}
	if alreadyRequested {
		return nil, ErrAlreadyRequested
	}

	return s.repo.CreateJoinRequest(ctx, teamID, playerID)
}

// GetJoinRequests returns all pending join requests for a team.
// Only the team owner or admin may view these.
func (s *TeamService) GetJoinRequests(ctx context.Context, requesterID, requesterRole, teamID string) ([]JoinRequest, error) {
	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("TeamService.GetJoinRequests: %w", err)
	}

	if requesterRole != "admin" && requesterRole != "moderator" && team.OwnerID != requesterID {
		return nil, ErrForbidden
	}

	requests, err := s.repo.GetPendingJoinRequests(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.GetJoinRequests: %w", err)
	}
	if requests == nil {
		requests = []JoinRequest{}
	}
	return requests, nil
}

// ResolveJoinRequest accepts or rejects a join request.
func (s *TeamService) ResolveJoinRequest(ctx context.Context, requesterID, requesterRole, teamID, requestID, action string) error {
	if action != "accept" && action != "reject" {
		return ErrInvalidAction
	}

	team, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTeamNotFound
		}
		return fmt.Errorf("TeamService.ResolveJoinRequest: %w", err)
	}
	if requesterRole != "admin" && team.OwnerID != requesterID {
		return ErrForbidden
	}

	req, err := s.repo.GetJoinRequest(ctx, requestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrJoinRequestNotFound
		}
		return fmt.Errorf("TeamService.ResolveJoinRequest: %w", err)
	}
	if req.TeamID != teamID {
		return ErrJoinRequestNotFound
	}
	if req.Status != "pending" {
		return ErrRequestNotPending
	}

	if action == "accept" {
		// Check the player isn't already on a team (race condition guard).
		alreadyMember, err := s.repo.IsMember(ctx, req.PlayerID)
		if err != nil {
			return fmt.Errorf("TeamService.ResolveJoinRequest: %w", err)
		}
		if alreadyMember {
			// Auto-reject instead of failing loudly.
			return s.repo.RejectJoinRequest(ctx, requestID)
		}
		return s.repo.AcceptJoinRequest(ctx, requestID, teamID, req.PlayerID)
	}

	return s.repo.RejectJoinRequest(ctx, requestID)
}

// AddArkadePoints adjusts a team's Arkade point total. Moderator/admin only.
func (s *TeamService) AddArkadePoints(ctx context.Context, requesterID, teamID string, req AddArkadePointsRequest) error {
	if req.Delta == 0 {
		return ErrInvalidDelta
	}

	_, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTeamNotFound
		}
		return fmt.Errorf("TeamService.AddArkadePoints: %w", err)
	}

	return s.repo.AddArkadePoints(ctx, teamID, requesterID, req.Delta, req.Reason)
}

// GetArkadePointHistory returns the point audit log for a team. Moderator/admin only.
func (s *TeamService) GetArkadePointHistory(ctx context.Context, teamID string) ([]ArkadePointLog, error) {
	_, err := s.repo.GetTeam(ctx, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTeamNotFound
		}
		return nil, fmt.Errorf("TeamService.GetArkadePointHistory: %w", err)
	}

	logs, err := s.repo.GetArkadePointHistory(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("TeamService.GetArkadePointHistory: %w", err)
	}
	if logs == nil {
		logs = []ArkadePointLog{}
	}
	return logs, nil
}

// validateTag checks that the tag is 2-10 chars, letters and digits only.
func validateTag(tag string) error {
	if len(tag) < 2 || len(tag) > 10 {
		return ErrInvalidTag
	}
	for _, ch := range tag {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			return ErrInvalidTag
		}
	}
	return nil
}
