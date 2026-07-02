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
	"log/slog"
	"time"

	// --- Third-party ---
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	// --- Internal ---
	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/events"
)

// Sentinel errors — named error values callers can check with errors.Is().
var (
	ErrInvalidClassRole     = errors.New("class_role must be one of: smartass, ninja, psycho, hacker, merkava, kommando")
	ErrSkillNotFound        = errors.New("one or more skill IDs do not exist")
	ErrSkillWrongClass      = errors.New("all skills must match your current class_role")
	ErrTierAboveLevel       = errors.New("skill tier exceeds your current level")
	ErrOneSkillPerTier      = errors.New("you can hold only one skill per tier — blue or red, never both")
	ErrInvalidLevel         = errors.New("level must be between 1 and 3")
	ErrGearNotFound         = errors.New("one or more gear type IDs do not exist")
	ErrGearClassRestricted  = errors.New("your class cannot equip this gear")
	ErrInvalidAmount        = errors.New("amount must be greater than 0")
	ErrInsufficientKredits  = errors.New("insufficient Kredit balance")
	ErrNoClassSet           = errors.New("you must set a class_role before allocating skills")
	ErrPlayerNotFound       = errors.New("player not found")
	ErrCannotTransferToSelf = errors.New("cannot transfer Kredits to yourself")
)

// validClassRoles is the set of allowed archetypes, matching the DB CHECK constraint.
var validClassRoles = map[string]bool{
	"smartass": true, "ninja": true, "psycho": true,
	"hacker": true, "merkava": true, "kommando": true,
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
	GetPlayerCore(ctx context.Context, playerID string) (classRole *string, level int, passiveHPBonus int, err error)
	SetPlayerLevel(ctx context.Context, playerID string, level int) error
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
	repo   Repository
	events events.Publisher // async event publisher — NoopPublisher unless wired via WithEvents
}

// NewPlayerService constructs a PlayerService.
// The repo parameter accepts the Repository interface — in production a
// *PlayerRepository is passed in; in unit tests a mock is used instead.
func NewPlayerService(repo Repository) *PlayerService {
	return &PlayerService{repo: repo, events: events.NoopPublisher{}}
}

// WithEvents attaches a NATS event publisher (used by player-service in
// microservice mode). Returns the service for chaining.
func (s *PlayerService) WithEvents(pub events.Publisher) *PlayerService {
	s.events = pub
	return s
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
// HP = BaseHP (3) + archetype passive bonus + permanent skill bonuses.
func (s *PlayerService) GetStats(ctx context.Context, playerID string) (*StatsResponse, error) {
	classRole, level, passiveHP, err := s.repo.GetPlayerCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetStats: %w", err)
	}

	resp := &StatsResponse{
		ClassRole: classRole,
		Level:     level,
	}

	// If the player hasn't chosen a class yet, return zeroed combat stats.
	if classRole == nil {
		return resp, nil
	}

	resp.HealthPoints = BaseHP + passiveHP

	// Add permanent bonuses from held skills (e.g. merkava's Fridge).
	skills, err := s.repo.GetPlayerSkills(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetStats: skills: %w", err)
	}
	for _, sk := range skills {
		resp.HealthPoints += sk.HPBonus
	}

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

// GetSkills returns the player's currently allocated skills and their level.
func (s *PlayerService) GetSkills(ctx context.Context, playerID string) (*SkillsResponse, error) {
	_, level, _, err := s.repo.GetPlayerCore(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetSkills: %w", err)
	}

	skills, err := s.repo.GetPlayerSkills(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("PlayerService.GetSkills: %w", err)
	}
	if skills == nil {
		skills = []Skill{}
	}

	return &SkillsResponse{
		AllocatedSkills: skills,
		Level:           level,
	}, nil
}

// SetSkills replaces the player's entire skill allocation.
// Rules (from the archetype sheet):
//   - every skill must belong to the player's archetype
//   - a skill of tier N requires player level >= N
//   - at most ONE skill per tier — blue or red, never both
func (s *PlayerService) SetSkills(ctx context.Context, playerID string, req SetSkillsRequest) error {
	classRole, level, _, err := s.repo.GetPlayerCore(ctx, playerID)
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

	// Verify all requested IDs were found (duplicates collapse and fail here too).
	if len(skills) != len(req.SkillIDs) {
		return ErrSkillNotFound
	}

	seenTier := map[int]string{} // tier → skill name already claiming it
	for _, sk := range skills {
		if sk.ClassRole != *classRole {
			return fmt.Errorf("%w: skill %q belongs to class %q, you are %q",
				ErrSkillWrongClass, sk.Name, sk.ClassRole, *classRole)
		}
		if sk.Tier > level {
			return fmt.Errorf("%w: %q is tier %d, you are level %d",
				ErrTierAboveLevel, sk.Name, sk.Tier, level)
		}
		if other, taken := seenTier[sk.Tier]; taken {
			return fmt.Errorf("%w: tier %d has both %q and %q",
				ErrOneSkillPerTier, sk.Tier, other, sk.Name)
		}
		seenTier[sk.Tier] = sk.Name
	}

	return s.repo.SetPlayerSkills(ctx, playerID, req.SkillIDs)
}

// SetLevel updates a player's level (moderator/admin only — enforced by
// middleware in the handler). Lowering a level prunes skill allocations
// whose tier is now out of reach.
func (s *PlayerService) SetLevel(ctx context.Context, playerID string, req SetLevelRequest) error {
	if req.Level < 1 || req.Level > MaxLevel {
		return ErrInvalidLevel
	}

	exists, err := s.repo.PlayerExists(ctx, playerID)
	if err != nil {
		return fmt.Errorf("PlayerService.SetLevel: %w", err)
	}
	if !exists {
		return ErrPlayerNotFound
	}

	if err := s.repo.SetPlayerLevel(ctx, playerID, req.Level); err != nil {
		return fmt.Errorf("PlayerService.SetLevel: %w", err)
	}
	return nil
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
// All requested gear type IDs must exist, and class-restricted items
// (spear/long weapon, shield, power balls) must be legal for the player's
// archetype. Gear can exceed the team budget (allowed but flagged red).
func (s *PlayerService) SetGear(ctx context.Context, playerID string, req SetGearRequest) error {
	if len(req.GearTypeIDs) > 0 {
		gear, err := s.repo.GetGearTypesByIDs(ctx, req.GearTypeIDs)
		if err != nil {
			return fmt.Errorf("PlayerService.SetGear: %w", err)
		}
		if len(gear) != len(req.GearTypeIDs) {
			return ErrGearNotFound
		}

		classRole, _, _, err := s.repo.GetPlayerCore(ctx, playerID)
		if err != nil {
			return fmt.Errorf("PlayerService.SetGear: %w", err)
		}
		for _, g := range gear {
			if len(g.RestrictedTo) == 0 {
				continue // unrestricted item
			}
			allowed := false
			if classRole != nil {
				for _, cls := range g.RestrictedTo {
					if cls == *classRole {
						allowed = true
						break
					}
				}
			}
			if !allowed {
				return fmt.Errorf("%w: %q is limited to %v", ErrGearClassRestricted, g.Name, g.RestrictedTo)
			}
		}
	}
	if err := s.repo.SetPlayerGear(ctx, playerID, req.GearTypeIDs); err != nil {
		return err
	}
	// Notify subscribers (team-service recomputes gear pool usage) — best-effort.
	if err := s.events.Publish(ctx, events.SubjectPlayerGearUpdated, events.PlayerGearUpdated{
		PlayerID: playerID, At: time.Now().UTC(),
	}); err != nil {
		slog.Error("PlayerService: event publish failed", "subject", events.SubjectPlayerGearUpdated, "error", err)
	}
	return nil
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
