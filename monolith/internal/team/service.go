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
	"log/slog"
	"strings"
	"time"
	"unicode"

	// --- Third-party ---
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	// --- Internal ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/events"
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

// Repository defines the data-access methods that TeamService needs.
//
// This interface lists every repository method the service calls. The concrete
// *TeamRepository satisfies it automatically via Go's structural typing.
// In unit tests, a mock struct with the same methods is used instead.
//
// See auth/service.go for a longer explanation of the interface pattern.
type Repository interface {
	ListTeams(ctx context.Context) ([]*Team, error)
	GetTeam(ctx context.Context, teamID string) (*Team, error)
	GetMembers(ctx context.Context, teamID string) ([]TeamMember, error)
	GetTeamGearUsed(ctx context.Context, teamID string) (int, error)
	GetOwnerUsername(ctx context.Context, ownerID string) (string, error)
	CreateTeam(ctx context.Context, name, tag, ownerID string) (*Team, error)
	UpdateTeam(ctx context.Context, teamID, name, tag string) error
	DeleteTeam(ctx context.Context, teamID, ownerID string) error
	ToggleLock(ctx context.Context, teamID string) (bool, error)
	RemoveMember(ctx context.Context, teamID, playerID string) error
	IsMember(ctx context.Context, playerID string) (bool, error)
	HasPendingRequest(ctx context.Context, teamID, playerID string) (bool, error)
	CreateJoinRequest(ctx context.Context, teamID, playerID string) (*JoinRequest, error)
	GetPendingJoinRequests(ctx context.Context, teamID string) ([]JoinRequest, error)
	GetJoinRequest(ctx context.Context, requestID string) (*JoinRequest, error)
	AcceptJoinRequest(ctx context.Context, requestID, teamID, playerID string) error
	RejectJoinRequest(ctx context.Context, requestID string) error
	AddArkadePoints(ctx context.Context, teamID, changedBy string, delta int, reason string) error
	GetArkadePointHistory(ctx context.Context, teamID string) ([]ArkadePointLog, error)
}

// TeamService contains all business logic for team operations.
type TeamService struct {
	repo   Repository
	events events.Publisher // async event publisher — NoopPublisher unless wired via WithEvents
}

// NewTeamService constructs a TeamService.
// The repo parameter accepts the Repository interface — in production a
// *TeamRepository is passed in; in unit tests a mock is used instead.
func NewTeamService(repo Repository) *TeamService {
	return &TeamService{repo: repo, events: events.NoopPublisher{}}
}

// WithEvents attaches a NATS event publisher (used by team-service in
// microservice mode). Returns the service for chaining.
func (s *TeamService) WithEvents(pub events.Publisher) *TeamService {
	s.events = pub
	return s
}

// publish emits an event, logging (not failing) on error — events are
// best-effort notifications, never part of the transaction.
func (s *TeamService) publish(ctx context.Context, subject string, payload any) {
	if err := s.events.Publish(ctx, subject, payload); err != nil {
		slog.Error("TeamService: event publish failed", "subject", subject, "error", err)
	}
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

	if err := s.repo.RemoveMember(ctx, teamID, playerID); err != nil {
		return err
	}
	s.publish(ctx, events.SubjectTeamMembershipChanged, events.TeamMembershipChanged{
		TeamID: teamID, PlayerID: playerID, Action: "removed", At: time.Now().UTC(),
	})
	return nil
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
		if err := s.repo.AcceptJoinRequest(ctx, requestID, teamID, req.PlayerID); err != nil {
			return err
		}
		s.publish(ctx, events.SubjectTeamMembershipChanged, events.TeamMembershipChanged{
			TeamID: teamID, PlayerID: req.PlayerID, Action: "joined", At: time.Now().UTC(),
		})
		return nil
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

	if err := s.repo.AddArkadePoints(ctx, teamID, requesterID, req.Delta, req.Reason); err != nil {
		return err
	}
	s.publish(ctx, events.SubjectTeamPointsUpdated, events.TeamPointsUpdated{
		TeamID: teamID, Delta: req.Delta, Reason: req.Reason, ChangedBy: requesterID, At: time.Now().UTC(),
	})
	return nil
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
