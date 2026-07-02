// Tests that PlayerService publishes NATS events on gear changes.
package player

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

func TestSetGear_PublishesGearUpdatedEvent(t *testing.T) {
	repo := &mockRepository{
		getGearTypesByIDsFn: func(ctx context.Context, ids []string) ([]GearType, error) {
			return []GearType{{ID: "gear-1"}}, nil
		},
		setPlayerGearFn: func(ctx context.Context, playerID string, gearTypeIDs []string) error { return nil },
	}
	pub := &capturePublisher{}
	svc := NewPlayerService(repo).WithEvents(pub)

	err := svc.SetGear(context.Background(), "player-1", SetGearRequest{GearTypeIDs: []string{"gear-1"}})
	if err != nil {
		t.Fatalf("SetGear returned error: %v", err)
	}

	if len(pub.subjects) != 1 || pub.subjects[0] != events.SubjectPlayerGearUpdated {
		t.Fatalf("expected 1 %s event, got %v", events.SubjectPlayerGearUpdated, pub.subjects)
	}
	evt := pub.payloads[0].(events.PlayerGearUpdated)
	if evt.PlayerID != "player-1" {
		t.Errorf("unexpected event payload: %+v", evt)
	}
}
