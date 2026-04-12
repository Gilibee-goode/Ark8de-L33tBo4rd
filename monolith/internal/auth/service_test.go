// Package auth — Unit tests for AuthService
//
// These tests verify the business logic in auth/service.go WITHOUT a database.
// Instead of a real *PlayerRepository (which needs PostgreSQL), we inject a
// hand-written mock that implements the Repository interface.
//
// WHAT IS A MOCK?
//   A mock is a fake implementation of an interface that you control in tests.
//   Instead of executing SQL, each method calls a function field that the test sets.
//   This lets you simulate any scenario: success, duplicate email, DB failure, etc.
//
// WHY HAND-WRITTEN MOCKS?
//   Go has mock generators (mockgen, moq) that create mock code automatically.
//   We use hand-written mocks here because they are completely transparent —
//   you can read the mock and understand exactly what it does without learning
//   a third-party library. Once you're comfortable, you can switch to generators.
//
// RUN THESE TESTS:
//   cd monolith && go test ./internal/auth/... -v
package auth

import (
	// --- Standard library ---
	"context" // context.Background() for all service calls
	"errors"  // errors.Is for checking sentinel errors
	"testing" // Go's built-in test framework

	// --- Third-party ---
	// pgx.ErrNoRows is the sentinel error the repo returns when a query finds nothing.
	// We use it in mocks to simulate "email not found" or "player not found".
	"github.com/jackc/pgx/v5"

	// pgconn.PgError is the PostgreSQL-specific error type we use to simulate
	// unique constraint violations (duplicate email/username).
	"github.com/jackc/pgx/v5/pgconn"

	// bcrypt is used to pre-compute a password hash for login tests.
	// In production we use DefaultCost (slow, secure); in tests we use MinCost (fast).
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Mock repository
// ---------------------------------------------------------------------------

// mockRepository is a fake implementation of the Repository interface.
//
// HOW IT WORKS:
//   Each method on the mock delegates to a function field. The test sets these
//   fields to control what the mock returns. For example, to simulate a
//   duplicate email error:
//
//     mock.createPlayerFn = func(...) (*Player, error) {
//         return nil, &pgconn.PgError{Code: "23505", ConstraintName: "players_email_key"}
//     }
//
//   Any method field left as nil will panic if called — this catches unexpected
//   calls that indicate the test's assumptions about the code path are wrong.
type mockRepository struct {
	createPlayerFn func(ctx context.Context, username, email, passwordHash string) (*Player, error)
	getByEmailFn   func(ctx context.Context, email string) (*Player, error)
	getByIDFn      func(ctx context.Context, id string) (*Player, error)
}

// CreatePlayer delegates to the function field set by the test.
func (m *mockRepository) CreatePlayer(ctx context.Context, username, email, passwordHash string) (*Player, error) {
	return m.createPlayerFn(ctx, username, email, passwordHash)
}

// GetByEmail delegates to the function field set by the test.
func (m *mockRepository) GetByEmail(ctx context.Context, email string) (*Player, error) {
	return m.getByEmailFn(ctx, email)
}

// GetByID delegates to the function field set by the test.
func (m *mockRepository) GetByID(ctx context.Context, id string) (*Player, error) {
	return m.getByIDFn(ctx, id)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testJWTSecret is the secret used to sign tokens in tests.
// It must match the secret passed to NewAuthService so tokens validate correctly.
const testJWTSecret = "test-secret-for-unit-tests"

// testPasswordHash is a bcrypt hash of "ValidPass1" — computed once and reused.
//
// WHY bcrypt.MinCost?
//   bcrypt is deliberately slow to resist brute-force attacks. DefaultCost (10)
//   takes ~100ms per hash. MinCost (4) takes <1ms — fast enough for tests.
//   We don't need security strength in unit tests, just a valid hash.
var testPasswordHash string

// init runs automatically before any test in this package.
// We use it to pre-compute the bcrypt hash because it's used in multiple tests.
func init() {
	hash, err := bcrypt.GenerateFromPassword([]byte("ValidPass1"), bcrypt.MinCost)
	if err != nil {
		panic("failed to generate test password hash: " + err.Error())
	}
	testPasswordHash = string(hash)
}

// makeTestPlayer returns a Player with reasonable defaults for testing.
// Tests can override specific fields after calling this.
func makeTestPlayer() *Player {
	return &Player{
		ID:               "test-player-id",
		Username:         "testuser",
		Email:            "test@example.com",
		PasswordHash:     testPasswordHash,
		Role:             "player",
		SkillPointsTotal: 20,
	}
}

// ---------------------------------------------------------------------------
// Register tests
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	mock := &mockRepository{
		createPlayerFn: func(ctx context.Context, username, email, passwordHash string) (*Player, error) {
			// Verify the service normalised the inputs.
			if username != "testuser" {
				t.Errorf("expected username 'testuser', got %q", username)
			}
			if email != "test@example.com" {
				t.Errorf("expected normalised email 'test@example.com', got %q", email)
			}
			// Verify a bcrypt hash was passed (not the raw password).
			if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("ValidPass1")); err != nil {
				t.Error("expected passwordHash to be a valid bcrypt hash of 'ValidPass1'")
			}
			return makeTestPlayer(), nil
		},
	}

	svc := NewAuthService(mock, testJWTSecret)
	resp, token, err := svc.Register(context.Background(), RegisterRequest{
		Username: "  testuser  ", // extra whitespace — should be trimmed
		Email:    "Test@Example.com", // mixed case — should be lowercased
		Password: "ValidPass1",
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty JWT token")
	}
	if resp.Username != "testuser" {
		t.Fatalf("expected username 'testuser', got %q", resp.Username)
	}
}

func TestRegister_InvalidUsername_TooShort(t *testing.T) {
	// The mock should never be called — validation fails before reaching the repo.
	svc := NewAuthService(&mockRepository{}, testJWTSecret)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "ab", // 2 chars — minimum is 3
		Email:    "test@example.com",
		Password: "ValidPass1",
	})

	if !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got: %v", err)
	}
}

func TestRegister_InvalidUsername_BadChars(t *testing.T) {
	svc := NewAuthService(&mockRepository{}, testJWTSecret)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "user@name", // @ is not allowed
		Email:    "test@example.com",
		Password: "ValidPass1",
	})

	if !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("expected ErrInvalidUsername, got: %v", err)
	}
}

func TestRegister_InvalidEmail(t *testing.T) {
	svc := NewAuthService(&mockRepository{}, testJWTSecret)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "notanemail", // no @ sign
		Password: "ValidPass1",
	})

	if !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("expected ErrInvalidEmail, got: %v", err)
	}
}

func TestRegister_WeakPassword_TooShort(t *testing.T) {
	svc := NewAuthService(&mockRepository{}, testJWTSecret)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "Ab1", // only 3 chars — minimum is 8
	})

	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got: %v", err)
	}
}

func TestRegister_WeakPassword_NoUppercase(t *testing.T) {
	svc := NewAuthService(&mockRepository{}, testJWTSecret)

	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "alllowercase1", // no uppercase letter
	})

	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("expected ErrWeakPassword, got: %v", err)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	mock := &mockRepository{
		createPlayerFn: func(ctx context.Context, username, email, passwordHash string) (*Player, error) {
			// Simulate PostgreSQL's unique constraint violation on the email column.
			// Error code "23505" = unique_violation in PostgreSQL.
			return nil, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "players_email_key",
			}
		},
	}

	svc := NewAuthService(mock, testJWTSecret)
	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "newuser",
		Email:    "taken@example.com",
		Password: "ValidPass1",
	})

	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got: %v", err)
	}
}

func TestRegister_DuplicateUsername(t *testing.T) {
	mock := &mockRepository{
		createPlayerFn: func(ctx context.Context, username, email, passwordHash string) (*Player, error) {
			return nil, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "players_username_key",
			}
		},
	}

	svc := NewAuthService(mock, testJWTSecret)
	_, _, err := svc.Register(context.Background(), RegisterRequest{
		Username: "taken_name",
		Email:    "unique@example.com",
		Password: "ValidPass1",
	})

	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_Success(t *testing.T) {
	mock := &mockRepository{
		getByEmailFn: func(ctx context.Context, email string) (*Player, error) {
			return makeTestPlayer(), nil
		},
	}

	svc := NewAuthService(mock, testJWTSecret)
	resp, token, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "ValidPass1", // matches testPasswordHash
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if token == "" {
		t.Fatal("expected a non-empty JWT token")
	}
	if resp.Username != "testuser" {
		t.Fatalf("expected username 'testuser', got %q", resp.Username)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	mock := &mockRepository{
		getByEmailFn: func(ctx context.Context, email string) (*Player, error) {
			return makeTestPlayer(), nil
		},
	}

	svc := NewAuthService(mock, testJWTSecret)
	_, _, err := svc.Login(context.Background(), LoginRequest{
		Email:    "test@example.com",
		Password: "WrongPassword1", // does not match the bcrypt hash
	})

	// Security: wrong password should return the same error as email not found.
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got: %v", err)
	}
}

func TestLogin_EmailNotFound(t *testing.T) {
	mock := &mockRepository{
		getByEmailFn: func(ctx context.Context, email string) (*Player, error) {
			// Simulate "no player with this email" — same as a SELECT that returns 0 rows.
			return nil, pgx.ErrNoRows
		},
	}

	svc := NewAuthService(mock, testJWTSecret)
	_, _, err := svc.Login(context.Background(), LoginRequest{
		Email:    "nobody@example.com",
		Password: "SomePass1",
	})

	// Security: same error as wrong password — never reveal if an email exists.
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got: %v", err)
	}
}
