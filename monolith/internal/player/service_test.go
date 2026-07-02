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
	getPlayerCoreFn         func(ctx context.Context, playerID string) (classRole *string, skillPointsTotal int, err error)
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
func (m *mockRepository) GetPlayerCore(ctx context.Context, playerID string) (*string, int, error) {
	return m.getPlayerCoreFn(ctx, playerID)
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
// Go doesn't allow taking the address of a string literal (&"tank" is illegal),
// so we use this helper to create *string values for test data.
func strPtr(s string) *string {
	return &s
}

// makeTestSkills returns a set of skills useful across multiple tests.
// These match the structure of real skills but with simple, predictable values.
func makeTestSkills() []Skill {
	return []Skill{
		{ID: "sk-1", Name: "Shield Wall", ClassRole: "tank", CostSkillPoints: 5, HPBonus: 20, ArmorBonus: 10},
		{ID: "sk-2", Name: "Taunt", ClassRole: "tank", CostSkillPoints: 3, HPBonus: 10, ArmorBonus: 5},
	}
}

// makeTestGear returns a set of gear types for testing.
func makeTestGear() []GearType {
	return []GearType{
		{ID: "g-1", Name: "Sword", GearPointCost: 2},
		{ID: "g-2", Name: "Shield", GearPointCost: 5},
	}
}

// ---------------------------------------------------------------------------
// SetClass tests
// ---------------------------------------------------------------------------

func TestSetClass_ValidRole(t *testing.T) {
	mock := &mockRepository{
		updateClassRoleFn: func(ctx context.Context, playerID, classRole string) error {
			if classRole != "tank" {
				t.Errorf("expected class 'tank', got %q", classRole)
			}
			return nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetClass(context.Background(), "player-1", "tank")
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
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			// nil classRole = player hasn't chosen a class yet.
			return nil, 20, nil
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
	if stats.ArmorPoints != 0 {
		t.Fatalf("expected 0 Armor, got %d", stats.ArmorPoints)
	}
	if stats.SkillPointsTotal != 20 {
		t.Fatalf("expected 20 skill points total, got %d", stats.SkillPointsTotal)
	}
}

func TestGetStats_WithSkillsAndGear(t *testing.T) {
	// When a player has a class, skills, and gear, GetStats computes accumulated values.
	tankClass := "tank"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			return &tankClass, 20, nil
		},
		getPlayerSkillsFn: func(ctx context.Context, playerID string) ([]Skill, error) {
			return makeTestSkills(), nil // sk-1: HP+20/Armor+10/Cost5, sk-2: HP+10/Armor+5/Cost3
		},
		getPlayerGearFn: func(ctx context.Context, playerID string) ([]GearType, error) {
			return makeTestGear(), nil // Sword: 2pts, Shield: 5pts
		},
	}

	svc := NewPlayerService(mock)
	stats, err := svc.GetStats(context.Background(), "player-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Tank base: HP=150, Armor=30. Skills add: HP+30, Armor+15. Total: HP=180, Armor=45.
	expectedHP := 150 + 20 + 10
	expectedArmor := 30 + 10 + 5
	if stats.HealthPoints != expectedHP {
		t.Fatalf("expected HP=%d, got %d", expectedHP, stats.HealthPoints)
	}
	if stats.ArmorPoints != expectedArmor {
		t.Fatalf("expected Armor=%d, got %d", expectedArmor, stats.ArmorPoints)
	}

	// Skill points: total 20, spent 5+3=8, remaining 12.
	if stats.SkillPointsRemaining != 12 {
		t.Fatalf("expected 12 skill points remaining, got %d", stats.SkillPointsRemaining)
	}

	// Gear points: 2+5=7.
	if stats.GearPointsUsed != 7 {
		t.Fatalf("expected 7 gear points used, got %d", stats.GearPointsUsed)
	}
}

// ---------------------------------------------------------------------------
// SetSkills tests
// ---------------------------------------------------------------------------

func TestSetSkills_Success(t *testing.T) {
	tankClass := "tank"
	var savedIDs []string

	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			return &tankClass, 20, nil
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			return makeTestSkills(), nil // 2 tank skills, total cost 8
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
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			// nil classRole — player hasn't chosen a class.
			return nil, 20, nil
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
	tankClass := "tank"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			return &tankClass, 20, nil
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
	dpsClass := "dps"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			return &dpsClass, 20, nil
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			// Return a tank skill for a DPS player — class mismatch.
			return makeTestSkills()[:1], nil // sk-1 is a tank skill
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

func TestSetSkills_BudgetExceeded(t *testing.T) {
	tankClass := "tank"
	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			// Only 5 skill points available.
			return &tankClass, 5, nil
		},
		getSkillsByIDsFn: func(ctx context.Context, ids []string) ([]Skill, error) {
			// Total cost = 5+3 = 8, exceeding the 5 point budget.
			return makeTestSkills(), nil
		},
	}

	svc := NewPlayerService(mock)
	err := svc.SetSkills(context.Background(), "player-1", SetSkillsRequest{
		SkillIDs: []string{"sk-1", "sk-2"},
	})
	if !errors.Is(err, ErrInsufficientSkillPts) {
		t.Fatalf("expected ErrInsufficientSkillPts, got: %v", err)
	}
}

func TestSetSkills_EmptyListClearsAllocation(t *testing.T) {
	tankClass := "tank"
	clearCalled := false

	mock := &mockRepository{
		getPlayerCoreFn: func(ctx context.Context, playerID string) (*string, int, error) {
			return &tankClass, 20, nil
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
// SetGear tests
// ---------------------------------------------------------------------------

func TestSetGear_Success(t *testing.T) {
	mock := &mockRepository{
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
			if classRole != "healer" {
				t.Errorf("expected class 'healer', got %q", classRole)
			}
			return []Skill{
				{ID: "sk-h1", Name: "Heal", ClassRole: "healer", CostSkillPoints: 4},
			}, nil
		},
	}

	svc := NewPlayerService(mock)
	skills, err := svc.GetAvailableSkills(context.Background(), "healer")
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
