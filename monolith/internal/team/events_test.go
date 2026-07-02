// Tests that TeamService publishes NATS events on state changes.
package team

import (
	"context"
	"testing"

	"github.com/Gilibee-goode/ark8de-l33tbo4rd/internal/events"
)

// capturePublisher records every published event for assertions.
type capturePublisher struct {
	subjects []string
	payloads []any
}

func (c *capturePublisher) Publish(_ context.Context, subject string, payload any) error {
	c.subjects = append(c.subjects, subject)
	c.payloads = append(c.payloads, payload)
	return nil
}

func TestAddArkadePoints_PublishesEvent(t *testing.T) {
	repo := &mockRepository{
		getTeamFn:         func(ctx context.Context, teamID string) (*Team, error) { return &Team{ID: teamID}, nil },
		addArkadePointsFn: func(ctx context.Context, teamID, changedBy string, delta int, reason string) error { return nil },
	}
	pub := &capturePublisher{}
	svc := NewTeamService(repo).WithEvents(pub)

	err := svc.AddArkadePoints(context.Background(), "mod-1", "team-1", AddArkadePointsRequest{Delta: 5, Reason: "won round"})
	if err != nil {
		t.Fatalf("AddArkadePoints returned error: %v", err)
	}

	if len(pub.subjects) != 1 || pub.subjects[0] != events.SubjectTeamPointsUpdated {
		t.Fatalf("expected 1 %s event, got %v", events.SubjectTeamPointsUpdated, pub.subjects)
	}
	evt := pub.payloads[0].(events.TeamPointsUpdated)
	if evt.TeamID != "team-1" || evt.Delta != 5 || evt.ChangedBy != "mod-1" {
		t.Errorf("unexpected event payload: %+v", evt)
	}
}

func TestResolveJoinRequest_Accept_PublishesMembershipEvent(t *testing.T) {
	repo := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return &Team{ID: teamID, OwnerID: "owner-1"}, nil
		},
		getJoinRequestFn: func(ctx context.Context, requestID string) (*JoinRequest, error) {
			return &JoinRequest{ID: requestID, TeamID: "team-1", PlayerID: "player-9", Status: "pending"}, nil
		},
		isMemberFn:          func(ctx context.Context, playerID string) (bool, error) { return false, nil },
		acceptJoinRequestFn: func(ctx context.Context, requestID, teamID, playerID string) error { return nil },
	}
	pub := &capturePublisher{}
	svc := NewTeamService(repo).WithEvents(pub)

	err := svc.ResolveJoinRequest(context.Background(), "owner-1", "team_owner", "team-1", "req-1", "accept")
	if err != nil {
		t.Fatalf("ResolveJoinRequest returned error: %v", err)
	}

	if len(pub.subjects) != 1 || pub.subjects[0] != events.SubjectTeamMembershipChanged {
		t.Fatalf("expected 1 %s event, got %v", events.SubjectTeamMembershipChanged, pub.subjects)
	}
	evt := pub.payloads[0].(events.TeamMembershipChanged)
	if evt.TeamID != "team-1" || evt.PlayerID != "player-9" || evt.Action != "joined" {
		t.Errorf("unexpected event payload: %+v", evt)
	}
}

func TestRemoveMember_PublishesMembershipEvent(t *testing.T) {
	repo := &mockRepository{
		getTeamFn: func(ctx context.Context, teamID string) (*Team, error) {
			return &Team{ID: teamID, OwnerID: "owner-1"}, nil
		},
		removeMemberFn: func(ctx context.Context, teamID, playerID string) error { return nil },
	}
	pub := &capturePublisher{}
	svc := NewTeamService(repo).WithEvents(pub)

	err := svc.RemoveMember(context.Background(), "owner-1", "team_owner", "team-1", "player-9")
	if err != nil {
		t.Fatalf("RemoveMember returned error: %v", err)
	}

	if len(pub.subjects) != 1 || pub.subjects[0] != events.SubjectTeamMembershipChanged {
		t.Fatalf("expected 1 %s event, got %v", events.SubjectTeamMembershipChanged, pub.subjects)
	}
	evt := pub.payloads[0].(events.TeamMembershipChanged)
	if evt.Action != "removed" {
		t.Errorf("expected action removed, got %+v", evt)
	}
}
