// Package player — Unit tests for PlayerService
//
// These tests verify the business logic in player/service.go WITHOUT a database.
// Instead of a real *PlayerRepository (which needs PostgreSQL), we inject a
// hand-written mock that implements the Repository interface.
//
// See auth/service_test.go for an in-depth explanation of hand-written mocks,
// function fields, and why we test this way.
//
// RUN THESE TESTS:
//   cd monolith && go test ./internal/player/... -v
package player

import (
	// --- Standard library ---
	"context" // context.Background() for all service calls
	"errors"  // errors.Is for checking sentinel errors
	"testing" // Go's built-in test framework

	// --- Third-party ---
	// pgconn.PgError is the PostgreSQL-specific error type we use to simulate
	// constraint violations (e.g. insufficient Kredits via CHECK constraint).
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// Mock repository
// ---------------------------------------------------------------------------

// mockRepository is a fake implementation of the Repository interface.
// Each method delegates to a function field that the test controls.
// See auth/service_test.go for a detailed explanation of this pattern.
type mockRepository struct {
	getPublicProfileFn      func(ctx context.Context, playerID string) (*PublicPlayerResponse, error)
	updateClassRoleFn       func(ctx context.Context, playerID, classRole string) error
	getPlayerCoreFn         func(ctx context.Context, playerID string) (classRole *string, level int, passiveHPBonus int, err error)
	setPlayerLevelFn        func(ctx context.Context, playerID string, level int) error
	getPlayerSkillsFn       func(ctx context.Context, playerID string) ([]Skill, error)
	getPlayerGearFn         func(ctx context.Context, playerID string) ([]GearType, error)
	getTeamGearPointsUsedFn func(ctx context.Context, playerID string) (teamTotal, teamUsed int, err error)
	getSkillsByIDsFn        func(ctx context.Context, ids []string) ([]Skill, error)
	setPlayerSkillsFn       func(ctx context.Context, playerID string, skillIDs []string) error
	getGearTypesByIDsFn     func(ctx context.Context, ids []string) ([]GearType, error)
	setPlayerGearFn         func(ctx context.Context, playerID string, gearTypeIDs []string) error
	getKreditBalanceFn      func(ctx context.Context, playerID string) (int, error)
	getKreditTransactionsFn func(ctx context.Context, playerID string) ([]KreditTransaction, error)
	playerExistsFn          func(ctx context.Context, playerID string) (bool, error)
	grantKreditsFn          func(ctx context.Context, toPlayerID, createdBy string, amount int, note string) error
	transferKreditsFn       func(ctx context.Context, fromPlayerID, toPlayerID, createdBy string, amount int, note string) error
	getSkillsByClassFn      func(ctx context.Context, classRole string) ([]Skill, error)
}

func (m *mockRepository) GetPublicProfile(ctx context.Context, playerID string) (*PublicPlayerResponse, error) {
	return m.getPublicProfileFn(ctx, playerID)
}
func (m *mockRepository) UpdateClassRole(ctx context.Context, playerID, classRole string) error {
	return m.updateClassRoleFn(ctx, playerID, classRole)
}
func (m *mockRepository) GetPlayerCore(ctx context.Context, playerID string) (*string, int, int, error) {
	return m.getPlayerCoreFn(ctx, playerID)
}
func (m *mockRepository) SetPlayerLevel(ctx context.Context, playerID string, level int) error {
	return m.setPlayerLevelFn(ctx, playerID, level)
}
func (m *mockRepository) GetPlayerSkills(ctx context.Context, playerID string) ([]Skill, error) {
	return m.getPlayerSkillsFn(ctx, playerID)
}
func (m *mockRepository) GetPlayerGear(ctx context.Context, playerID string) ([]GearType, error) {
	return m.getPlayerGearFn(ctx, playerID)
}
func (m *mockRepository) GetTeamGearPointsUsed(ctx context.Context, playerID string) (int, int, error) {
	return m.getTeamGearPointsUsedFn(ctx, playerID)
}
func (m *mockRepository) GetSkillsByIDs(ctx context.Context, ids []string) ([]Skill, error) {
	return m.getSkillsByIDsFn(ctx, ids)
}
func (m *mockRepository) SetPlayerSkills(ctx context.Context, playerID string, skillIDs []string) error {
	return m.setPlayerSkillsFn(ctx, playerID, skillIDs)
}
func (m *mockRepository) GetGearTypesByIDs(ctx context.Context, ids []string) ([]GearType, error) {
	return m.getGearTypesByIDsFn(ctx, ids)
}
func (m *mockRepository) SetPlayerGear(ctx context.Context, playerID string, gearTypeIDs []string) error {
	return m.setPlayerGearFn(ctx, playerID, gearTypeIDs)
}
func (m *mockRepository) GetKreditBalance(ctx context.Context, playerID string) (int, error) {
	return m.getKreditBalanceFn(ctx, playerID)
}
func (m *mockRepository) GetKreditTransactions(ctx context.Context, playerID string) ([]KreditTransaction, error) {
	return m.getKreditTransactionsFn(ctx, playerID)
}
func (m *mockRepository) PlayerExists(ctx context.Context, playerID string) (bool, error) {
	return m.playerExistsFn(ctx, playerID)
}
func (m *mockRepository) GrantKredits(ctx context.Context, toPlayerID, createdBy string, amount int, note string) error {
	return m.grantKreditsFn(ctx, toPlayerID, createdBy, amount, note)
}
func (m *mockRepository) TransferKredits(ctx context.Context, fromPlayerID, toPlayerID, createdBy string, amount int, note string) error {
	return m.transferKreditsFn(ctx, fromPlayerID, toPlayerID, createdBy, amount, note)
}
func (m *mockRepository) GetSkillsByClass(ctx context.Context, classRole string) ([]Skill, error) {
	return m.getSkillsByClassFn(ctx, classRole)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// strPtr is a tiny helper that returns a pointer to a string.
// Go doesn't allow taking the address of a string literal (&"merkava" is illegal),
// so we use this helper to create *string values for test data.
func strPtr(s string) *string {
	return &s
}

// makeTestSkills returns two merkava skills (blue branch, tiers 1–2).
// Fridge is the only skill in the game with a permanent HP bonus.
func makeTestSkills() []Skill {
	return []Skill{
		{ID: "sk-1", ClassRole: "merkava", Branch: "blue", Tier: 1, Name: "Fridge", HPBonus: 1},
		{ID: "sk-2", ClassRole: "merkava", Branch: "blue", Tier: 2, Name: "Windbreaker"},
	}
}

// makeTestGear returns one unrestricted weapon and one class-restricted one.
func makeTestGear() []GearType {
	return []GearType{
		{ID: "g-1", Name: "dagger", GearPointCost: 1},
		{ID: "g-2", Name: "shield", GearPointCost: 4,
			RestrictedTo: []string{"psycho", "hacker", "merkava", "kommando"}},
	}
}

// ---------------------------------------------------------------------------
// SetClass tests
// ---------------------------------------------------------------------------

func TestSetClass_ValidRole(t *testing.T) {
	mock := &mockRepository{
		updateClassRoleFn: func(ctx context.Context, playerID, classRole string) error {
			if classRole != "merkava" {
				t.Errorf("expected class 'merkava', got %q", classRole)
			}
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetClass(context.Background(), "player-1", "merkava")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSetClass_InvalidRole(t *testing.T) {
	// The mock should never be called — validation fails before reaching the repo.
	svc := NewPlayerService(&mockRepository{})

	err := svc.SetClass(context.Background(), "player-1", "wizard")
	if !errors.Is(err, ErrInvalidClassRole) {
		t.Fatalf("expected ErrInvalidClassRole, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetStats tests
// ---------------------------------------------------------------------------

func TestGetStats_NoClassSet(t *testing.T) {
	// When a player hasn't chosen a class yet, GetStats returns zeroed combat stats.
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			// nil classRole = player hasn't chosen a class yet.
			return nil, 1, 0, nil
		},
	}

	svc := NewPlayerService(mock)
	stats, err := svc.GetStats(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if stats.ClassRole != nil {
		t.Fatalf("expected nil ClassRole, got %v", *stats.ClassRole)
	}
	if stats.HealthPoints != 0 {
		t.Fatalf("expected 0 HP, got %d", stats.HealthPoints)
	}
	if stats.Level != 1 {
		t.Fatalf("expected level 1, got %d", stats.Level)
	}
}

func TestGetStats_WithSkillsAndGear(t *testing.T) {
	// HP = BaseHP (3) + archetype passive (merkava: +1) + skill bonuses (Fridge: +1).
	merkava := "merkava"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 2, 1, nil // level 2, passive +1 HP
		},
		getPlayerSkillsFn: func(ctx context.Context, playerID string) ([]Skill, error) {
			return makeTestSkills(), nil // Fridge (+1 HP), Windbreaker (+0)
		},
		getPlayerGearFn: func(ctx context.Context, playerID string) ([]GearType, error) {
			return makeTestGear(), nil // dagger: 1pt, shield: 4pts
		},
	}

	svc := NewPlayerService(mock)
	stats, err := svc.GetStats(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	expectedHP := BaseHP + 1 + 1 // base 3 + merkava passive + Fridge
	if stats.HealthPoints != expectedHP {
		t.Fatalf("expected HP=%d, got %d", expectedHP, stats.HealthPoints)
	}
	if stats.Level != 2 {
		t.Fatalf("expected level 2, got %d", stats.Level)
	}

	// Gear points: 1+4=5.
	if stats.GearPointsUsed != 5 {
		t.Fatalf("expected 5 gear points used, got %d", stats.GearPointsUsed)
	}
}

// ---------------------------------------------------------------------------
// SetSkills tests
// ---------------------------------------------------------------------------

func TestSetSkills_Success(t *testing.T) {
	merkava := "merkava"
	var savedIDs []string

	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 2, 1, nil // level 2 — may hold tiers 1 and 2
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			return makeTestSkills(), nil // tier 1 + tier 2, both merkava
		},
		setPlayerSkillsFn: func(ctx context.Context, playerID string, skillIDs []string) error {
			savedIDs = skillIDs
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-1", "sk-2"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(savedIDs) != 2 {
		t.Fatalf("expected 2 skill IDs saved, got %d", len(savedIDs))
	}
}

func TestSetSkills_NoClassSet(t *testing.T) {
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			// nil classRole — player hasn't chosen a class.
			return nil, 1, 0, nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-1"},
	})
	if !errors.Is(err, ErrNoClassSet) {
		t.Fatalf("expected ErrNoClassSet, got: %v", err)
	}
}

func TestSetSkills_SkillNotFound(t *testing.T) {
	merkava := "merkava"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 3, 1, nil
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			// Return fewer skills than requested — simulates "skill not found".
			return []Skill{makeTestSkills()[0]}, nil // only 1 of the 2 requested
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-1", "sk-nonexistent"},
	})
	if !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("expected ErrSkillNotFound, got: %v", err)
	}
}

func TestSetSkills_WrongClass(t *testing.T) {
	ninja := "ninja"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &ninja, 3, 0, nil
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			// Return a merkava skill for a ninja player — class mismatch.
			return makeTestSkills()[:1], nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-1"},
	})
	if !errors.Is(err, ErrSkillWrongClass) {
		t.Fatalf("expected ErrSkillWrongClass, got: %v", err)
	}
}

func TestSetSkills_TierAboveLevel(t *testing.T) {
	merkava := "merkava"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 1, 1, nil // level 1 — tier 2 is out of reach
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			return makeTestSkills()[1:], nil // Windbreaker, tier 2
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-2"},
	})
	if !errors.Is(err, ErrTierAboveLevel) {
		t.Fatalf("expected ErrTierAboveLevel, got: %v", err)
	}
}

func TestSetSkills_OneSkillPerTier(t *testing.T) {
	merkava := "merkava"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 3, 1, nil
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			// Both branches of tier 1 — blue AND red is illegal.
			return []Skill{
				{ID: "sk-1", ClassRole: "merkava", Branch: "blue", Tier: 1, Name: "Fridge", HPBonus: 1},
				{ID: "sk-3", ClassRole: "merkava", Branch: "red", Tier: 1, Name: "Nailed to the Floor"},
			}, nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-1", "sk-3"},
	})
	if !errors.Is(err, ErrOneSkillPerTier) {
		t.Fatalf("expected ErrOneSkillPerTier, got: %v", err)
	}
}

func TestSetSkills_EmptyListClearsAllocation(t *testing.T) {
	merkava := "merkava"
	clearCalled := false

	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 2, 1, nil
		},
		setPlayerSkillsFn: func(ctx context.Context, playerID string, skillIDs []string) error {
			clearCalled = true
			if skillIDs != nil {
				t.Errorf("expected nil skillIDs for clear, got %v", skillIDs)
			}
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{}, // empty list = clear all skills
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !clearCalled {
		t.Fatal("expected SetPlayerSkills to be called to clear allocations")
	}
}

// ---------------------------------------------------------------------------
// SetLevel tests
// ---------------------------------------------------------------------------

func TestSetLevel_Success(t *testing.T) {
	var savedLevel int
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) { return true, nil },
		setPlayerLevelFn: func(ctx context.Context, playerID string, level int) error {
			savedLevel = level
			return nil
		},
	}

	svc := NewPlayerService(mock)
	if err := svc.SetLevel(context.Background(), "player-1", SetLevelRequest{Level: 3}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if savedLevel != 3 {
		t.Fatalf("expected level 3 saved, got %d", savedLevel)
	}
}

func TestSetLevel_InvalidLevel(t *testing.T) {
	svc := NewPlayerService(&mockRepository{})

	for _, lvl := range []int{0, 4, -1} {
		err := svc.SetLevel(context.Background(), "player-1", SetLevelRequest{Level: lvl})
		if !errors.Is(err, ErrInvalidLevel) {
			t.Fatalf("level %d: expected ErrInvalidLevel, got: %v", lvl, err)
		}
	}
}

func TestSetLevel_PlayerNotFound(t *testing.T) {
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) { return false, nil },
	}

	svc := NewPlayerService(mock)
	err := svc.SetLevel(context.Background(), "ghost", SetLevelRequest{Level: 2})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SetGear tests
// ---------------------------------------------------------------------------

func TestSetGear_Success(t *testing.T) {
	merkava := "merkava"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &merkava, 1, 1, nil // merkava may equip the shield
		},
		getGearTypesByIDsFn: func(ctx context.Context, ids []string) ([]GearType, error) {
			return makeTestGear(), nil
		},
		setPlayerGearFn: func(ctx context.Context, playerID string, gearTypeIDs []string) error {
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetGear(context.Background(), "player-1", SetGearRequest{
		GearTypeIDs: []string{"g-1", "g-2"},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSetGear_GearNotFound(t *testing.T) {
	mock := &mockRepository{
		getGearTypesByIDsFn: func(ctx context.Context, ids []string) ([]GearType, error) {
			// Return fewer gear types than requested — simulates "gear not found".
			return makeTestGear()[:1], nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetGear(context.Background(), "player-1", SetGearRequest{
		GearTypeIDs: []string{"g-1", "g-nonexistent"},
	})
	if !errors.Is(err, ErrGearNotFound) {
		t.Fatalf("expected ErrGearNotFound, got: %v", err)
	}
}

func TestSetGear_ClassRestricted(t *testing.T) {
	// A ninja may not equip a shield (restricted to psycho/hacker/merkava/kommando).
	ninja := "ninja"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, int, error) {
			return &ninja, 1, 0, nil
		},
		getGearTypesByIDsFn: func(ctx context.Context, ids []string) ([]GearType, error) {
			return makeTestGear()[1:], nil // the shield
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetGear(context.Background(), "player-1", SetGearRequest{
		GearTypeIDs: []string{"g-2"},
	})
	if !errors.Is(err, ErrGearClassRestricted) {
		t.Fatalf("expected ErrGearClassRestricted, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TransferKredits tests
// ---------------------------------------------------------------------------

func TestTransferKredits_Success(t *testing.T) {
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) {
			return true, nil
		},
		transferKreditsFn: func(ctx context.Context, fromPlayerID, toPlayerID, createdBy string, amount int, note string) error {
			if fromPlayerID != "sender-1" {
				t.Errorf("expected from 'sender-1', got %q", fromPlayerID)
			}
			if toPlayerID != "receiver-1" {
				t.Errorf("expected to 'receiver-1', got %q", toPlayerID)
			}
			if amount != 50 {
				t.Errorf("expected amount 50, got %d", amount)
			}
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.TransferKredits(context.Background(), "sender-1", TransferKreditsRequest{
		ToPlayerID: "receiver-1",
		Amount:     50,
		Note:       "good game",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestTransferKredits_SelfTransfer(t *testing.T) {
	// No repo calls needed — validation fails before reaching the repo.
	svc := NewPlayerService(&mockRepository{})

	err := svc.TransferKredits(context.Background(), "player-1", TransferKreditsRequest{
		ToPlayerID: "player-1", // same as sender
		Amount:     10,
	})
	if !errors.Is(err, ErrCannotTransferToSelf) {
		t.Fatalf("expected ErrCannotTransferToSelf, got: %v", err)
	}
}

func TestTransferKredits_ZeroAmount(t *testing.T) {
	svc := NewPlayerService(&mockRepository{})

	err := svc.TransferKredits(context.Background(), "player-1", TransferKreditsRequest{
		ToPlayerID: "player-2",
		Amount:     0,
	})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got: %v", err)
	}
}

func TestTransferKredits_RecipientNotFound(t *testing.T) {
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) {
			return false, nil // recipient doesn't exist
		},
	}

	svc := NewPlayerService(mock)
	err := svc.TransferKredits(context.Background(), "sender-1", TransferKreditsRequest{
		ToPlayerID: "ghost-player",
		Amount:     10,
	})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound, got: %v", err)
	}
}

func TestTransferKredits_InsufficientBalance(t *testing.T) {
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) {
			return true, nil
		},
		transferKreditsFn: func(ctx context.Context, fromPlayerID, toPlayerID, createdBy string, amount int, note string) error {
			// Simulate PostgreSQL's CHECK constraint violation on the kredits column.
			// Error code "23514" = check_violation in PostgreSQL.
			// This happens when the sender's balance would go below 0.
			return &pgconn.PgError{Code: "23514"}
		},
	}

	svc := NewPlayerService(mock)
	err := svc.TransferKredits(context.Background(), "broke-player", TransferKreditsRequest{
		ToPlayerID: "receiver-1",
		Amount:     9999,
	})
	if !errors.Is(err, ErrInsufficientKredits) {
		t.Fatalf("expected ErrInsufficientKredits, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GrantKredits tests
// ---------------------------------------------------------------------------

func TestGrantKredits_Success(t *testing.T) {
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) {
			return true, nil
		},
		grantKreditsFn: func(ctx context.Context, toPlayerID, createdBy string, amount int, note string) error {
			if amount != 100 {
				t.Errorf("expected amount 100, got %d", amount)
			}
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.GrantKredits(context.Background(), "player-1", "moderator-1", GrantKreditsRequest{
		Amount: 100,
		Note:   "prize for event",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestGrantKredits_ZeroAmount(t *testing.T) {
	svc := NewPlayerService(&mockRepository{})

	err := svc.GrantKredits(context.Background(), "player-1", "moderator-1", GrantKreditsRequest{
		Amount: 0,
	})
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("expected ErrInvalidAmount, got: %v", err)
	}
}

func TestGrantKredits_PlayerNotFound(t *testing.T) {
	mock := &mockRepository{
		playerExistsFn: func(ctx context.Context, playerID string) (bool, error) {
			return false, nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.GrantKredits(context.Background(), "ghost-player", "moderator-1", GrantKreditsRequest{
		Amount: 100,
	})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetAvailableSkills tests
// ---------------------------------------------------------------------------

func TestGetAvailableSkills_ValidClass(t *testing.T) {
	mock := &mockRepository{
		getSkillsByClassFn: func(ctx context.Context, classRole string) ([]Skill, error) {
			if classRole != "psycho" {
				t.Errorf("expected class 'psycho', got %q", classRole)
			}
			return []Skill{
				{ID: "sk-p1", ClassRole: "psycho", Branch: "blue", Tier: 1, Name: "Basic Psychosis"},
			}, nil
		},
	}

	svc := NewPlayerService(mock)
	skills, err := svc.GetAvailableSkills(context.Background(), "psycho")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
}

func TestGetAvailableSkills_InvalidClass(t *testing.T) {
	svc := NewPlayerService(&mockRepository{})

	_, err := svc.GetAvailableSkills(context.Background(), "wizard")
	if !errors.Is(err, ErrInvalidClassRole) {
		t.Fatalf("expected ErrInvalidClassRole, got: %v", err)
	}
}
