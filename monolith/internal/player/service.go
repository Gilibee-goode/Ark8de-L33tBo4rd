// Package player — Business Logic / Service layer
//
// Business rules enforced here:
//   - class_role must be one of the four valid values
//   - Skills must match the player's class before allocation
//   - Total skill cost must not exceed skill_points_total
//   - Gear type IDs must be valid (exist in gear_types table)
//   - Kredit amounts must be positive
//   - Players cannot transfer more Kredits than they hold
//   - Only moderators/admins can grant Kredits to other players
package player

import (
	// --- Standard library ---
	"context"
	"errors"
	"fmt"

	// --- Third-party ---
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Sentinel errors — named error values callers can check with errors.Is().
var (
	ErrInvalidClassRole      = errors.New("class_role must be one of: tank, dps, healer, support")
	ErrSkillNotFound         = errors.New("one or more skill IDs do not exist")
	ErrSkillWrongClass       = errors.New("all skills must match your current class_role")
	ErrInsufficientSkillPts  = errors.New("not enough skill points — reduce your skill selection")
	ErrGearNotFound          = errors.New("one or more gear type IDs do not exist")
	ErrInvalidAmount         = errors.New("amount must be greater than 0")
	ErrInsufficientKredits   = errors.New("insufficient Kredit balance")
	ErrNoClassSet            = errors.New("you must set a class_role before allocating skills")
	ErrPlayerNotFound        = errors.New("player not found")
	ErrCannotTransferToSelf  = errors.New("cannot transfer Kredits to yourself")
)

// validClassRoles is the set of allowed class_role values, matching the DB CHECK constraint.
var validClassRoles = map[string]bool{
	"tank": true, "dps": true, "healer": true, "support": true,
}

// Repository defines the data-access methods that PlayerService needs.
//
// This interface lists every repository method the service calls. The concrete
// *PlayerRepository satisfies it automatically via Go's structural typing —
// no "implements" keyword needed. In unit tests, a mock struct with the same
// methods is used instead, letting us test business logic without a database.
//
// See auth/service.go for a longer explanation of why we use interfaces here.
type Repository interface {
	GetPublicProfile(ctx context.Context, playerID string) (*PublicPlayerResponse, error)
	UpdateClassRole(ctx context.Context, playerID, classRole string) error
	GetPlayerCore(ctx context.Context, playerID string) (classRole *string, skillPointsTotal int, err error)
	GetPlayerSkills(ctx context.Context, playerID string) ([]Skill, error)
	GetPlayerGear(ctx context.Context, playerID string) ([]GearType, error)
	GetTeamGearPointsUsed(ctx context.Context, playerID string) (teamTotal, teamUsed int, err error)
	GetSkillsByIDs(ctx context.Context, ids []string) ([]Skill, error)
	SetPlayerSkills(ctx context.Context, playerID string, skillIDs []string) error
	GetGearTypesByIDs(ctx context.Context, ids []string) ([]GearType, error)
	SetPlayerGear(ctx context.Context, playerID string, gearTypeIDs []string) error
	GetKreditBalance(ctx context.Context, playerID string) (int, error)
	GetKreditTransactions(ctx context.Context, playerID string) ([]KreditTransaction, error)
	PlayerExists(ctx context.Context, playerID string) (bool, error)
	GrantKredits(ctx context.Context, toPlayerID, createdBy string, amount int, note string) error
	TransferKredits(ctx context.Context, fromPlayerID, toPlayerID, createdBy string, amount int, note string) error
	GetSkillsByClass(ctx context.Context, classRole string) ([]Skill, error)
}

// PlayerService contains all business logic for player operations.
type PlayerService struct {
	repo Repository
}

// NewPlayerService constructs a PlayerService.
// The repo parameter accepts the Repository interface — in production a
// *PlayerRepository is passed in; in unit tests a mock is used instead.
func NewPlayerService(repo Repository) *PlayerService {
	return &PlayerService{repo: repo}
}

// GetPublicProfile returns the publicly visible profile for any player.
func (s *PlayerService) GetPublicProfile(ctx context.Context, playerID string) (*PublicPlayerResponse, error) {
	p, err := s.repo.GetPublicProfile(ctx, playerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlayerNotFound
		}
		return nil, fmt.Errorf("PlayerService.GetPublicProfile: %w", err)
	}
	return p, nil
}

// SetClass updates the player's class_role and clears their skill allocations.
// Changing class is allowed at any time (even after skills have been allocated),
// but the old skills are wiped because they're class-specific.
func (s *PlayerService) SetClass(ctx context.Context, playerID, classRole string) error {
	if !validClassRoles[classRole] {
		return ErrInvalidClassRole
	}
	if err := s.repo.UpdateClassRole(ctx, playerID, classRole); err != nil {
		return fmt.Errorf("PlayerService.SetClass: %w", err)
	}
	return nil
}

// GetStats computes and returns a player's current combat stats.
// Nothing is stored — these are always derived from the current DB state.
func (s *PlayerService) GetStats(ctx context.Context, playerID string) (*StatsResponse, error) {
	classRole, skillPointsTotal, err := s.repo.GetPlayerCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetStats: %w", err)
	}

	resp := &StatsResponse{
		ClassRole:        classRole,
		SkillPointsTotal: skillPointsTotal,
	}

	// If the player hasn't chosen a class yet, return zeroed combat stats.
	if classRole == nil {
		return resp, nil
	}

	// Base stats from class.
	resp.HealthPoints = classBaseHP[*classRole]
	resp.ArmorPoints = classBaseArmor[*classRole]

	// Add bonuses from allocated skills.
	skills, err := s.repo.GetPlayerSkills(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetStats: skills: %w", err)
	}
	spentPoints := 0
	for _, sk := range skills {
		resp.HealthPoints += sk.HPBonus
		resp.ArmorPoints += sk.ArmorBonus
		spentPoints += sk.CostSkillPoints
	}
	resp.SkillPointsRemaining = skillPointsTotal - spentPoints

	// Gear points used by this player (their own gear selections only).
	gear, err := s.repo.GetPlayerGear(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetStats: gear: %w", err)
	}
	for _, g := range gear {
		resp.GearPointsUsed += g.GearPointCost
	}

	return resp, nil
}

// GetSkills returns the player's currently allocated skills and their remaining point budget.
func (s *PlayerService) GetSkills(ctx context.Context, playerID string) (*SkillsResponse, error) {
	classRole, skillPointsTotal, err := s.repo.GetPlayerCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetSkills: %w", err)
	}

	skills, err := s.repo.GetPlayerSkills(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetSkills: %w", err)
	}

	spentPoints := 0
	for _, sk := range skills {
		spentPoints += sk.CostSkillPoints
	}

	_ = classRole // available for future validation
	return &SkillsResponse{
		AllocatedSkills:      skills,
		SkillPointsTotal:     skillPointsTotal,
		SkillPointsRemaining: skillPointsTotal - spentPoints,
	}, nil
}

// SetSkills replaces the player's entire skill allocation.
// All requested skills must exist, belong to the player's class, and fit within the budget.
func (s *PlayerService) SetSkills(ctx context.Context, playerID string, req SetSkillsRequest) error {
	classRole, skillPointsTotal, err := s.repo.GetPlayerCore(ctx, playerID)
	if err != nil {
		return fmt.Errorf("PlayerService.SetSkills: %w", err)
	}
	if classRole == nil {
		return ErrNoClassSet
	}

	// If the request is empty, clear all allocations.
	if len(req.SkillIDs) == 0 {
		return s.repo.SetPlayerSkills(ctx, playerID, nil)
	}

	// Fetch the requested skills to validate them.
	skills, err := s.repo.GetSkillsByIDs(ctx, req.SkillIDs)
	if err != nil {
		return fmt.Errorf("PlayerService.SetSkills: fetch skills: %w", err)
	}

	// Verify all requested IDs were found.
	if len(skills) != len(req.SkillIDs) {
		return ErrSkillNotFound
	}

	// Verify all skills match the player's current class.
	totalCost := 0
	for _, sk := range skills {
		if sk.ClassRole != *classRole {
			return fmt.Errorf("%w: skill %q belongs to class %q, you are %q",
				ErrSkillWrongClass, sk.Name, sk.ClassRole, *classRole)
		}
		totalCost += sk.CostSkillPoints
	}

	// Verify the player has enough skill points.
	if totalCost > skillPointsTotal {
		return fmt.Errorf("%w: need %d, have %d", ErrInsufficientSkillPts, totalCost, skillPointsTotal)
	}

	return s.repo.SetPlayerSkills(ctx, playerID, req.SkillIDs)
}

// GetGear returns the player's current gear selections and team gear pool info.
func (s *PlayerService) GetGear(ctx context.Context, playerID string) (*GearResponse, error) {
	gear, err := s.repo.GetPlayerGear(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetGear: %w", err)
	}

	myGearCost := 0
	for _, g := range gear {
		myGearCost += g.GearPointCost
	}

	teamTotal, teamUsed, err := s.repo.GetTeamGearPointsUsed(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetGear: team gear: %w", err)
	}

	return &GearResponse{
		SelectedGear:         gear,
		GearPointsUsedByMe:   myGearCost,
		TeamGearPointsTotal:  teamTotal,
		TeamGearPointsUsed:   teamUsed,
		TeamGearPoolNegative: teamTotal > 0 && teamUsed > teamTotal,
	}, nil
}

// SetGear replaces the player's entire gear selection.
// All requested gear type IDs must exist. Gear can exceed the team budget
// (it's allowed but flagged as negative on the UI).
func (s *PlayerService) SetGear(ctx context.Context, playerID string, req SetGearRequest) error {
	if len(req.GearTypeIDs) > 0 {
		gear, err := s.repo.GetGearTypesByIDs(ctx, req.GearTypeIDs)
		if err != nil {
			return fmt.Errorf("PlayerService.SetGear: %w", err)
		}
		if len(gear) != len(req.GearTypeIDs) {
			return ErrGearNotFound
		}
	}
	return s.repo.SetPlayerGear(ctx, playerID, req.GearTypeIDs)
}

// GetKredits returns the player's Kredit balance and transaction history.
func (s *PlayerService) GetKredits(ctx context.Context, playerID string) (*KreditsResponse, error) {
	balance, err := s.repo.GetKreditBalance(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetKredits: %w", err)
	}

	txns, err := s.repo.GetKreditTransactions(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetKredits: %w", err)
	}

	// Return an empty slice (not nil) so the JSON response is [] not null.
	if txns == nil {
		txns = []KreditTransaction{}
	}

	return &KreditsResponse{Balance: balance, Transactions: txns}, nil
}

// GrantKredits adds Kredits to a target player. Only moderators/admins may call this.
// The role check is enforced in the handler via middleware — the service trusts it.
func (s *PlayerService) GrantKredits(ctx context.Context, toPlayerID, moderatorID string, req GrantKreditsRequest) error {
	if req.Amount <= 0 {
		return ErrInvalidAmount
	}

	exists, err := s.repo.PlayerExists(ctx, toPlayerID)
	if err != nil {
		return fmt.Errorf("PlayerService.GrantKredits: %w", err)
	}
	if !exists {
		return ErrPlayerNotFound
	}

	return s.repo.GrantKredits(ctx, toPlayerID, moderatorID, req.Amount, req.Note)
}

// TransferKredits moves Kredits from the sender to the target player.
// The sender cannot transfer more than their current balance.
func (s *PlayerService) TransferKredits(ctx context.Context, fromPlayerID string, req TransferKreditsRequest) error {
	if req.Amount <= 0 {
		return ErrInvalidAmount
	}
	if req.ToPlayerID == fromPlayerID {
		return ErrCannotTransferToSelf
	}

	exists, err := s.repo.PlayerExists(ctx, req.ToPlayerID)
	if err != nil {
		return fmt.Errorf("PlayerService.TransferKredits: %w", err)
	}
	if !exists {
		return ErrPlayerNotFound
	}

	err = s.repo.TransferKredits(ctx, fromPlayerID, req.ToPlayerID, fromPlayerID, req.Amount, req.Note)
	if err != nil {
		// A check violation (code 23514) on the kredits column means the sender
		// tried to go below 0 — insufficient balance.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23514" {
			return ErrInsufficientKredits
		}
		return fmt.Errorf("PlayerService.TransferKredits: %w", err)
	}
	return nil
}

// GetAvailableSkills returns all skills for a given class role.
// Publicly accessible — no auth required.
func (s *PlayerService) GetAvailableSkills(ctx context.Context, classRole string) ([]Skill, error) {
	if classRole != "" && !validClassRoles[classRole] {
		return nil, ErrInvalidClassRole
	}
	skills, err := s.repo.GetSkillsByClass(ctx, classRole)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetAvailableSkills: %w", err)
	}
	if skills == nil {
		skills = []Skill{}
	}
	return skills, nil
}
