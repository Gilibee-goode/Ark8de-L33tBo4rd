// Package session — Unit tests for SessionService.
//
// These tests verify the business logic of session management in isolation,
// using a mock repository instead of a real database. This means:
//   - Tests run in ~1ms (no DB connection, no Docker)
//   - Each test controls exactly what the repository returns
//   - We test service logic (ID generation, expiry checking) not SQL
package session

import (
	// --- Standard library ---
	"context" // for passing context to service methods
	"errors"  // for creating test errors
	"testing" // Go's built-in test framework
	"time"    // for creating expired/valid session timestamps
)

// ---------------------------------------------------------------------------
// Mock repository
// ---------------------------------------------------------------------------

// mockRepository is a test double for the Repository interface.
// Each method is a function field that the test can set to control behaviour.
//
// WHY FUNCTION FIELDS (not a struct with hardcoded returns)?
//   Function fields give each test full control. Test A can make GetByID
//   return a valid session; test B can make it return ErrSessionNotFound.
//   No shared mutable state between tests.
type mockRepository struct {
	CreateFn           func(ctx context.Context, s *Session) error
	GetByIDFn          func(ctx context.Context, id string) (*Session, error)
	DeleteFn           func(ctx context.Context, id string) error
	DeleteByPlayerIDFn func(ctx context.Context, playerID string) error
	DeleteExpiredFn    func(ctx context.Context) error
}

// Each method delegates to its function field. If the field is nil, the test
// forgot to set it up — panic with a clear message so the bug is obvious.
func (m *mockRepository) Create(ctx context.Context, s *Session) error {
	if m.CreateFn == nil {
		panic("mockRepository.Create called but not set up")
	}
	return m.CreateFn(ctx, s)
}

func (m *mockRepository) GetByID(ctx context.Context, id string) (*Session, error) {
	if m.GetByIDFn == nil {
		panic("mockRepository.GetByID called but not set up")
	}
	return m.GetByIDFn(ctx, id)
}

func (m *mockRepository) Delete(ctx context.Context, id string) error {
	if m.DeleteFn == nil {
		panic("mockRepository.Delete called but not set up")
	}
	return m.DeleteFn(ctx, id)
}

func (m *mockRepository) DeleteByPlayerID(ctx context.Context, playerID string) error {
	if m.DeleteByPlayerIDFn == nil {
		panic("mockRepository.DeleteByPlayerID called but not set up")
	}
	return m.DeleteByPlayerIDFn(ctx, playerID)
}

func (m *mockRepository) DeleteExpired(ctx context.Context) error {
	if m.DeleteExpiredFn == nil {
		panic("mockRepository.DeleteExpired called but not set up")
	}
	return m.DeleteExpiredFn(ctx)
}

// Compile-time check that mockRepository satisfies the Repository interface.
var _ Repository = (*mockRepository)(nil)

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateSession_Success(t *testing.T) {
	mock := &mockRepository{
		CreateFn: func(ctx context.Context, s *Session) error {
			// Verify the session has reasonable values.
			if s.ID == "" {
				t.Error("expected non-empty session ID")
			}
			if len(s.ID) != 64 { // 32 bytes hex-encoded = 64 chars
				t.Errorf("expected 64-char hex ID, got %d chars", len(s.ID))
			}
			if s.PlayerID != "player-123" {
				t.Errorf("expected player-123, got %s", s.PlayerID)
			}
			if s.Role != "admin" {
				t.Errorf("expected admin role, got %s", s.Role)
			}
			if s.Username != "testuser" {
				t.Errorf("expected testuser, got %s", s.Username)
			}
			if s.ExpiresAt.Before(time.Now().Add(6 * 24 * time.Hour)) {
				t.Error("expected expiry at least 6 days from now")
			}
			return nil
		},
	}

	svc := NewSessionService(mock)
	sess, err := svc.CreateSession(context.Background(), "player-123", "admin", "testuser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.ID == "" {
		t.Error("expected session to have an ID")
	}
	if sess.PlayerID != "player-123" {
		t.Errorf("expected player-123, got %s", sess.PlayerID)
	}
}

func TestCreateSession_RepoError(t *testing.T) {
	mock := &mockRepository{
		CreateFn: func(ctx context.Context, s *Session) error {
			return errors.New("database is down")
		},
	}

	svc := NewSessionService(mock)
	_, err := svc.CreateSession(context.Background(), "player-123", "player", "testuser")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetSession_ValidSession(t *testing.T) {
	mock := &mockRepository{
		GetByIDFn: func(ctx context.Context, id string) (*Session, error) {
			return &Session{
				ID:        "abc123",
				PlayerID:  "player-456",
				Role:      "moderator",
				Username:  "moduser",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				ExpiresAt: time.Now().Add(6 * 24 * time.Hour), // still valid
			}, nil
		},
	}

	svc := NewSessionService(mock)
	sess, err := svc.GetSession(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess.PlayerID != "player-456" {
		t.Errorf("expected player-456, got %s", sess.PlayerID)
	}
	if sess.Username != "moduser" {
		t.Errorf("expected moduser, got %s", sess.Username)
	}
}

func TestGetSession_ExpiredSession(t *testing.T) {
	deleteCalled := false
	mock := &mockRepository{
		GetByIDFn: func(ctx context.Context, id string) (*Session, error) {
			return &Session{
				ID:        "expired-session",
				PlayerID:  "player-789",
				Role:      "player",
				Username:  "olduser",
				CreatedAt: time.Now().Add(-8 * 24 * time.Hour),
				ExpiresAt: time.Now().Add(-1 * time.Hour), // expired 1 hour ago
			}, nil
		},
		// The service should try to clean up the expired session.
		DeleteFn: func(ctx context.Context, id string) error {
			deleteCalled = true
			return nil
		},
	}

	svc := NewSessionService(mock)
	_, err := svc.GetSession(context.Background(), "expired-session")
	if !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if !deleteCalled {
		t.Error("expected Delete to be called for expired session cleanup")
	}
}

func TestGetSession_NotFound(t *testing.T) {
	mock := &mockRepository{
		GetByIDFn: func(ctx context.Context, id string) (*Session, error) {
			return nil, ErrSessionNotFound
		},
	}

	svc := NewSessionService(mock)
	_, err := svc.GetSession(context.Background(), "nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestDestroySession_Success(t *testing.T) {
	deletedID := ""
	mock := &mockRepository{
		DeleteFn: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	svc := NewSessionService(mock)
	err := svc.DestroySession(context.Background(), "sess-to-delete")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedID != "sess-to-delete" {
		t.Errorf("expected sess-to-delete, got %s", deletedID)
	}
}

func TestDestroyAllSessions_Success(t *testing.T) {
	deletedPlayerID := ""
	mock := &mockRepository{
		DeleteByPlayerIDFn: func(ctx context.Context, playerID string) error {
			deletedPlayerID = playerID
			return nil
		},
	}

	svc := NewSessionService(mock)
	err := svc.DestroyAllSessions(context.Background(), "player-999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deletedPlayerID != "player-999" {
		t.Errorf("expected player-999, got %s", deletedPlayerID)
	}
}

func TestCleanExpired_Success(t *testing.T) {
	cleanCalled := false
	mock := &mockRepository{
		DeleteExpiredFn: func(ctx context.Context) error {
			cleanCalled = true
			return nil
		},
	}

	svc := NewSessionService(mock)
	err := svc.CleanExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cleanCalled {
		t.Error("expected DeleteExpired to be called")
	}
}
