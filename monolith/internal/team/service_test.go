// Package team — Unit tests for TeamService
//
// These tests verify the business logic in team/service.go WITHOUT a database.
// Instead of a real *TeamRepository (which needs PostgreSQL), we inject a
// hand-written mock that implements the Repository interface.
//
// See auth/service_test.go for an in-depth explanation of hand-written mocks,
// function fields, and why we test this way.
//
// RUN THESE TESTS:
//   cd monolith && go test ./internal/team/... -v
package team

import (
	// --- Standard library ---
	"context" // context.Background() for all service calls
	"errors"  // errors.Is for checking sentinel errors
	"testing" // Go's built-in test framework
	"time"    // time.Now() for timestamp fields in test data

	// --- Third-party ---
	// pgx.ErrNoRows is the sentinel error the repo returns when a query finds nothing.
	"github.com/jackc/pgx/v5"

	// pgconn.PgError is the PostgreSQL-specific error type we use to simulate
	// unique constraint violations (duplicate team name or tag).
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// Mock repository
// ---------------------------------------------------------------------------

// mockRepository is a fake implementation of the Repository interface.
// Each method delegates to a function field that the test controls.
type mockRepository struct {
	listTeamsFn           func(ctx context.Context) ([]*Team, error)
	getTeamFn             func(ctx context.Context, teamID string) (*Team, error)
	getMembersFn          func(ctx context.Context, teamID string) ([]TeamMember, error)
	getTeamGearUsedFn     func(ctx context.Context, teamID string) (int, error)
	getOwnerUsernameFn    func(ctx context.Context, ownerID string) (string, error)
	createTeamFn          func(ctx context.Context, name, tag, ownerID string) (*Team, error)
	updateTeamFn          func(ctx context.Context, teamID, name, tag string) error
	deleteTeamFn          func(ctx context.Context, teamID, ownerID string) error
	toggleLockFn          func(ctx context.Context, teamID string) (bool, error)
	removeMemberFn        func(ctx context.Context, teamID, playerID string) error
	isMemberFn            func(ctx context.Context, playerID string) (bool, error)
	hasPendingRequestFn   func(ctx context.Context, teamID, playerID string) (bool, error)
	createJoinRequestFn   func(ctx context.Context, teamID, playerID string) (*JoinRequest, error)
	getPendingJoinReqsFn  func(ctx context.Context, teamID string) ([]JoinRequest, error)
	getJoinRequestFn      func(ctx context.Context, requestID string) (*JoinRequest, error)
	acceptJoinRequestFn   func(ctx context.Context, requestID, teamID, playerID string) error
	rejectJoinRequestFn   func(ctx context.Context, requestID string) error
	addArkadePointsFn     func(ctx context.Context, teamID, changedBy string, delta int, reason string) error
	getArkadePointHistFn  func(ctx context.Context, teamID string) ([]ArkadePointLog, error)
}

func (m *mockRepository) ListTeams(ctx context.Context) ([]*Team, error) {
	return m.listTeamsFn(ctx)
}
func (m *mockRepository) GetTeam(ctx context.Context, teamID string) (*Team, error) {
	return m.getTeamFn(ctx, teamID)
}
func (m *mockRepository) GetMembers(ctx context.Context, teamID string) ([]TeamMember, error) {
	return m.getMembersFn(ctx, teamID)
}
func (m *mockRepository) GetTeamGearUsed(ctx context.Context, teamID string) (int, error) {
	return m.getTeamGearUsedFn(ctx, teamID)
}
func (m *mockRepository) GetOwnerUsername(ctx context.Context, ownerID string) (string, error) {
	return m.getOwnerUsernameFn(ctx, ownerID)
}
func (m *mockRepository) CreateTeam(ctx context.Context, name, tag, ownerID string) (*Team, error) {
	return m.createTeamFn(ctx, name, tag, ownerID)
}
func (m *mockRepository) UpdateTeam(ctx context.Context, teamID, name, tag string) error {
	return m.updateTeamFn(ctx, teamID, name, tag)
}
func (m *mockRepository) DeleteTeam(ctx context.Context, teamID, ownerID string) error {
	return m.deleteTeamFn(ctx, teamID, ownerID)
}
func (m *mockRepository) ToggleLock(ctx context.Context, teamID string) (bool, error) {
	return m.toggleLockFn(ctx, teamID)
}
func (m *mockRepository) RemoveMember(ctx context.Context, teamID, playerID string) error {
	return m.removeMemberFn(ctx, teamID, playerID)
}
func (m *mockRepository) IsMember(ctx context.Context, playerID string) (bool, error) {
	return m.isMemberFn(ctx, playerID)
}
func (m *mockRepository) HasPendingRequest(ctx context.Context, teamID, playerID string) (bool, error) {
	return m.hasPendingRequestFn(ctx, teamID, playerID)
}
func (m *mockRepository) CreateJoinRequest(ctx context.Context, teamID, playerID string) (*JoinRequest, error) {
	return m.createJoinRequestFn(ctx, teamID, playerID)
}
func (m *mockRepository) GetPendingJoinRequests(ctx context.Context, teamID string) ([]JoinRequest, error) {
	return m.getPendingJoinReqsFn(ctx, teamID)
}
func (m *mockRepository) GetJoinRequest(ctx context.Context, requestID string) (*JoinRequest, error) {
	return m.getJoinRequestFn(ctx, requestID)
}
func (m *mockRepository) AcceptJoinRequest(ctx context.Context, requestID, teamID, playerID string) error {
	return m.acceptJoinRequestFn(ctx, requestID, teamID, playerID)
}
func (m *mockRepository) RejectJoinRequest(ctx context.Context, requestID string) error {
	return m.rejectJoinRequestFn(ctx, requestID)
}
func (m *mockRepository) AddArkadePoints(ctx context.Context, teamID, changedBy string, delta int, reason string) error {
	return m.addArkadePointsFn(ctx, teamID, changedBy, delta, reason)
}
func (m *mockRepository) GetArkadePointHistory(ctx context.Context, teamID string) ([]ArkadePointLog, error) {
	return m.getArkadePointHistFn(ctx, teamID)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// makeTestTeam returns a Team with reasonable defaults for testing.
func makeTestTeam() *Team {
	return &Team{
		ID:              "team-1",
		Name:            "Test Team",
		Tag:             "TST",
		OwnerID:         "owner-1",
		ArkadePoints:    100,
		GearPointsTotal: 12,
		IsLockedIn:      false,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// makeLockedTeam returns a team that has been locked in.
func makeLockedTeam() *Team {
	t := makeTestTeam()
	t.IsLockedIn = true
	return t
}

// makeTestJoinRequest returns a pending join request with reasonable defaults.
func makeTestJoinRequest() *JoinRequest {
	return &JoinRequest{
		ID:          "req-1",
		TeamID:      "team-1",
		PlayerID:    "joiner-1",
		Username:    "joiner",
		Status:      "pending",
		RequestedAt: time.Now(),
	}
}

// ---------------------------------------------------------------------------
// CreateTeam tests
// ---------------------------------------------------------------------------

func TestCreateTeam_Success(t *testing.T) {
	mock := &mockRepository{
		createTeamFn: func(ctx context.Context, name, tag, ownerID string) (*Team, error) {
			// Verify the service normalised the inputs.
			if name != "My Team" {
				t.Errorf("expected name 'My Team', got %q", name)
			}
			if tag != "MYT" {
				t.Errorf("expected tag 'MYT' (uppercased), got %q", tag)
			}
			return makeTestTeam(), nil
		},
	}

	svc := NewTeamService(mock)
	team, err := svc.CreateTeam(context.Background(), "owner-1", CreateTeamRequest{
		Name: "  My Team  ", // whitespace — should be trimmed
		Tag:  "myt",         // lowercase — should be uppercased
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if team == nil {
		t.Fatal("expected a team, got nil")
	}
}

func TestCreateTeam_NameTooShort(t *testing.T) {
	svc := NewTeamService(&mockRepository{})

	_, err := svc.CreateTeam(context.Background(), "owner-1", CreateTeamRequest{
		Name: "AB", // 2 chars — minimum is 3
		Tag:  "ABC",
	})
	if !errors.Is(err, ErrInvalidTeamName) {
		t.Fatalf("expected ErrInvalidTeamName, got: %v", err)
	}
}

func TestCreateTeam_InvalidTag(t *testing.T) {
	svc := NewTeamService(&mockRepository{})

	_, err := svc.CreateTeam(context.Background(), "owner-1", CreateTeamRequest{
		Name: "Valid Name",
		Tag:  "A", // 1 char — minimum is 2
	})
	if !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("expected ErrInvalidTag, got: %v", err)
	}
}

func TestCreateTeam_TagWithSpecialChars(t *testing.T) {
	svc := NewTeamService(&mockRepository{})

	_, err := svc.CreateTeam(context.Background(), "owner-1", CreateTeamRequest{
		Name: "Valid Name",
		Tag:  "A@B", // @ is not allowed — letters and digits only
	})
	if !errors.Is(err, ErrInvalidTag) {
		t.Fatalf("expected ErrInvalidTag, got: %v", err)
	}
}

func TestCreateTeam_DuplicateName(t *testing.T) {
	mock := &mockRepository{
		createTeamFn: func(ctx context.Context, name, tag, ownerID string) (*Team, error) {
			// Simulate PostgreSQL unique constraint violation on team name.
			return nil, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "teams_name_key",
			}
		},
	}

	svc := NewTeamService(mock)
	_, err := svc.CreateTeam(context.Background(), "owner-1", CreateTeamRequest{
		Name: "Taken Name",
		Tag:  "TKN",
	})
	if !errors.Is(err, ErrTeamNameTaken) {
		t.Fatalf("expected ErrTeamNameTaken, got: %v", err)
	}
}

func TestCreateTeam_DuplicateTag(t *testing.T) {
	mock := &mockRepository{
		createTeamFn: func(ctx context.Context, name, tag, ownerID string) (*Team, error) {
			return nil, &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "teams_tag_key",
			}
		},
	}

	svc := NewTeamService(mock)
	_, err := svc.CreateTeam(context.Background(), "owner-1", CreateTeamRequest{
		Name: "Unique Name",
		Tag:  "DUP",
	})
	if !errors.Is(err, ErrTagTaken) {
		t.Fatalf("expected ErrTagTaken, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateTeam tests
// ---------------------------------------------------------------------------

func TestUpdateTeam_OwnerSuccess(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
		updateTeamFn: func(ctx context.Context, teamID, name, tag string) error {
			return nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.UpdateTeam(context.Background(), "owner-1", "player", "team-1", UpdateTeamRequest{
		Name: "New Name",
		Tag:  "NEW",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestUpdateTeam_AdminSuccess(t *testing.T) {
	// An admin can update any team, even one they don't own.
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
		updateTeamFn: func(ctx context.Context, teamID, name, tag string) error {
			return nil
		},
	}

	svc := NewTeamService(mock)
	// requesterID is NOT the owner, but role is "admin" — should be allowed.
	err := svc.UpdateTeam(context.Background(), "admin-1", "admin", "team-1", UpdateTeamRequest{
		Name: "Admin Edit",
		Tag:  "ADM",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestUpdateTeam_Forbidden(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
	}

	svc := NewTeamService(mock)
	// requesterID is neither the owner nor an admin.
	err := svc.UpdateTeam(context.Background(), "rando-1", "player", "team-1", UpdateTeamRequest{
		Name: "Hijack",
		Tag:  "HAX",
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

func TestUpdateTeam_NotFound(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return nil, pgx.ErrNoRows
		},
	}

	svc := NewTeamService(mock)
	err := svc.UpdateTeam(context.Background(), "owner-1", "player", "nonexistent", UpdateTeamRequest{
		Name: "Whatever",
		Tag:  "WTV",
	})
	if !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("expected ErrTeamNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteTeam tests
// ---------------------------------------------------------------------------

func TestDeleteTeam_OwnerSuccess(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		deleteTeamFn: func(ctx context.Context, teamID, ownerID string) error {
			return nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.DeleteTeam(context.Background(), "owner-1", "player", "team-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDeleteTeam_Forbidden(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.DeleteTeam(context.Background(), "rando-1", "player", "team-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ToggleLock tests
// ---------------------------------------------------------------------------

func TestToggleLock_OwnerSuccess(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		toggleLockFn: func(ctx context.Context, teamID string) (bool, error) {
			return true, nil // returns the new lock state
		},
	}

	svc := NewTeamService(mock)
	locked, err := svc.ToggleLock(context.Background(), "owner-1", "player", "team-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !locked {
		t.Fatal("expected locked=true after toggle")
	}
}

func TestToggleLock_Forbidden(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
	}

	svc := NewTeamService(mock)
	_, err := svc.ToggleLock(context.Background(), "rando-1", "player", "team-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RemoveMember tests
// ---------------------------------------------------------------------------

func TestRemoveMember_Success(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
		removeMemberFn: func(ctx context.Context, teamID, playerID string) error {
			return nil
		},
	}

	svc := NewTeamService(mock)
	// Owner removes a regular member — should succeed.
	err := svc.RemoveMember(context.Background(), "owner-1", "player", "team-1", "member-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRemoveMember_CannotRemoveOwner(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
	}

	svc := NewTeamService(mock)
	// Trying to remove the owner themselves — should be blocked.
	err := svc.RemoveMember(context.Background(), "owner-1", "player", "team-1", "owner-1")
	if !errors.Is(err, ErrCannotRemoveOwner) {
		t.Fatalf("expected ErrCannotRemoveOwner, got: %v", err)
	}
}

func TestRemoveMember_Forbidden(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
	}

	svc := NewTeamService(mock)
	// A random player (not owner, not admin) tries to remove a member.
	err := svc.RemoveMember(context.Background(), "rando-1", "player", "team-1", "member-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SendJoinRequest tests
// ---------------------------------------------------------------------------

func TestSendJoinRequest_Success(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // not locked
		},
		isMemberFn: func(ctx context.Context, playerID string) (bool, error) {
			return false, nil // not already on a team
		},
		hasPendingRequestFn: func(ctx context.Context, teamID, playerID string) (bool, error) {
			return false, nil // no duplicate request
		},
		createJoinRequestFn: func(ctx context.Context, teamID, playerID string) (*JoinRequest, error) {
			return makeTestJoinRequest(), nil
		},
	}

	svc := NewTeamService(mock)
	req, err := svc.SendJoinRequest(context.Background(), "joiner-1", "team-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if req == nil {
		t.Fatal("expected a join request, got nil")
	}
}

func TestSendJoinRequest_TeamLocked(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeLockedTeam(), nil // team is locked in
		},
	}

	svc := NewTeamService(mock)
	_, err := svc.SendJoinRequest(context.Background(), "joiner-1", "team-1")
	if !errors.Is(err, ErrTeamLocked) {
		t.Fatalf("expected ErrTeamLocked, got: %v", err)
	}
}

func TestSendJoinRequest_AlreadyMember(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		isMemberFn: func(ctx context.Context, playerID string) (bool, error) {
			return true, nil // already on a team
		},
	}

	svc := NewTeamService(mock)
	_, err := svc.SendJoinRequest(context.Background(), "joiner-1", "team-1")
	if !errors.Is(err, ErrAlreadyInTeam) {
		t.Fatalf("expected ErrAlreadyInTeam, got: %v", err)
	}
}

func TestSendJoinRequest_AlreadyRequested(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		isMemberFn: func(ctx context.Context, playerID string) (bool, error) {
			return false, nil
		},
		hasPendingRequestFn: func(ctx context.Context, teamID, playerID string) (bool, error) {
			return true, nil // already has a pending request
		},
	}

	svc := NewTeamService(mock)
	_, err := svc.SendJoinRequest(context.Background(), "joiner-1", "team-1")
	if !errors.Is(err, ErrAlreadyRequested) {
		t.Fatalf("expected ErrAlreadyRequested, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ResolveJoinRequest tests
// ---------------------------------------------------------------------------

func TestResolveJoinRequest_AcceptSuccess(t *testing.T) {
	acceptCalled := false
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		getJoinRequestFn: func(ctx context.Context, requestID string) (*JoinRequest, error) {
			return makeTestJoinRequest(), nil // status = "pending", teamID = "team-1"
		},
		isMemberFn: func(ctx context.Context, playerID string) (bool, error) {
			return false, nil // not already on a team — accept is safe
		},
		acceptJoinRequestFn: func(ctx context.Context, requestID, teamID, playerID string) error {
			acceptCalled = true
			return nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.ResolveJoinRequest(context.Background(), "owner-1", "player", "team-1", "req-1", "accept")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !acceptCalled {
		t.Fatal("expected AcceptJoinRequest to be called")
	}
}

func TestResolveJoinRequest_RejectSuccess(t *testing.T) {
	rejectCalled := false
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		getJoinRequestFn: func(ctx context.Context, requestID string) (*JoinRequest, error) {
			return makeTestJoinRequest(), nil
		},
		rejectJoinRequestFn: func(ctx context.Context, requestID string) error {
			rejectCalled = true
			return nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.ResolveJoinRequest(context.Background(), "owner-1", "player", "team-1", "req-1", "reject")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !rejectCalled {
		t.Fatal("expected RejectJoinRequest to be called")
	}
}

func TestResolveJoinRequest_InvalidAction(t *testing.T) {
	svc := NewTeamService(&mockRepository{})

	err := svc.ResolveJoinRequest(context.Background(), "owner-1", "player", "team-1", "req-1", "maybe")
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected ErrInvalidAction, got: %v", err)
	}
}

func TestResolveJoinRequest_Forbidden(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
	}

	svc := NewTeamService(mock)
	// A random player (not owner, not admin) tries to resolve a request.
	err := svc.ResolveJoinRequest(context.Background(), "rando-1", "player", "team-1", "req-1", "accept")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

func TestResolveJoinRequest_NotPending(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		getJoinRequestFn: func(ctx context.Context, requestID string) (*JoinRequest, error) {
			req := makeTestJoinRequest()
			req.Status = "accepted" // already resolved
			return req, nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.ResolveJoinRequest(context.Background(), "owner-1", "player", "team-1", "req-1", "accept")
	if !errors.Is(err, ErrRequestNotPending) {
		t.Fatalf("expected ErrRequestNotPending, got: %v", err)
	}
}

func TestResolveJoinRequest_AcceptRaceGuard(t *testing.T) {
	// If the player joined another team between requesting and being accepted,
	// the service should auto-reject the request instead of failing loudly.
	rejectCalled := false
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		getJoinRequestFn: func(ctx context.Context, requestID string) (*JoinRequest, error) {
			return makeTestJoinRequest(), nil
		},
		isMemberFn: func(ctx context.Context, playerID string) (bool, error) {
			return true, nil // player joined another team in the meantime
		},
		rejectJoinRequestFn: func(ctx context.Context, requestID string) error {
			rejectCalled = true
			return nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.ResolveJoinRequest(context.Background(), "owner-1", "player", "team-1", "req-1", "accept")
	if err != nil {
		t.Fatalf("expected no error (auto-reject), got: %v", err)
	}
	if !rejectCalled {
		t.Fatal("expected RejectJoinRequest to be called as race-guard auto-reject")
	}
}

// ---------------------------------------------------------------------------
// AddArkadePoints tests
// ---------------------------------------------------------------------------

func TestAddArkadePoints_Success(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		addArkadePointsFn: func(ctx context.Context, teamID, changedBy string, delta int, reason string) error {
			if delta != 50 {
				t.Errorf("expected delta 50, got %d", delta)
			}
			return nil
		},
	}

	svc := NewTeamService(mock)
	err := svc.AddArkadePoints(context.Background(), "mod-1", "team-1", AddArkadePointsRequest{
		Delta:  50,
		Reason: "won the round",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestAddArkadePoints_ZeroDelta(t *testing.T) {
	svc := NewTeamService(&mockRepository{})

	err := svc.AddArkadePoints(context.Background(), "mod-1", "team-1", AddArkadePointsRequest{
		Delta: 0,
	})
	if !errors.Is(err, ErrInvalidDelta) {
		t.Fatalf("expected ErrInvalidDelta, got: %v", err)
	}
}

func TestAddArkadePoints_TeamNotFound(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return nil, pgx.ErrNoRows
		},
	}

	svc := NewTeamService(mock)
	err := svc.AddArkadePoints(context.Background(), "mod-1", "nonexistent", AddArkadePointsRequest{
		Delta:  10,
		Reason: "test",
	})
	if !errors.Is(err, ErrTeamNotFound) {
		t.Fatalf("expected ErrTeamNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetJoinRequests tests (authorization)
// ---------------------------------------------------------------------------

func TestGetJoinRequests_OwnerSuccess(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil
		},
		getPendingJoinReqsFn: func(ctx context.Context, teamID string) ([]JoinRequest, error) {
			return []JoinRequest{*makeTestJoinRequest()}, nil
		},
	}

	svc := NewTeamService(mock)
	requests, err := svc.GetJoinRequests(context.Background(), "owner-1", "player", "team-1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
}

func TestGetJoinRequests_Forbidden(t *testing.T) {
	mock := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return makeTestTeam(), nil // OwnerID = "owner-1"
		},
	}

	svc := NewTeamService(mock)
	// A regular player who is not the owner should be denied.
	_, err := svc.GetJoinRequests(context.Background(), "rando-1", "player", "team-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListTeams tests
// ---------------------------------------------------------------------------

func TestListTeams_ReturnsEmptySlice(t *testing.T) {
	// When the repo returns nil, the service should return an empty slice (not nil).
	// This ensures JSON serialisation produces [] instead of null.
	mock := &mockRepository{
		listTeamsFn: func(ctx context.Context) ([]*Team, error) {
			return nil, nil
		},
	}

	svc := NewTeamService(mock)
	teams, err := svc.ListTeams(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if teams == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(teams) != 0 {
		t.Fatalf("expected 0 teams, got %d", len(teams))
	}
}
