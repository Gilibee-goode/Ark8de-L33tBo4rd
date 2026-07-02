// NATS JetStream implementation of the events.Publisher interface,
// plus a subscription helper for consumer services.
package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// StreamName is the single JetStream stream that carries all Ark8de events.
const StreamName = "ARK8DE"

// streamSubjects are the subject patterns captured by the stream.
var streamSubjects = []string{"team.>", "player.>"}

// NATSPublisher publishes events to a JetStream stream.
type NATSPublisher struct {
	nc *nats.Conn
	js jetstream.JetStream
}

// ConnectNATS connects to NATS at url, ensures the ARK8DE stream exists,
// and returns a ready publisher. Callers should Close() it on shutdown.
func ConnectNATS(url string) (*NATSPublisher, error) {
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("events.ConnectNATS: connect to %s: %w", url, err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("events.ConnectNATS: jetstream init: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Idempotent: creates the stream on first boot, updates it otherwise.
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: streamSubjects,
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("events.ConnectNATS: ensure stream %s: %w", StreamName, err)
	}

	slog.Info("connected to NATS JetStream", "url", url, "stream", StreamName)
	return &NATSPublisher{nc: nc, js: js}, nil
}

// Publish serializes payload as JSON and publishes it to subject.
func (p *NATSPublisher) Publish(ctx context.Context, subject string, payload any) error {
	data, err := marshal(payload)
	if err != nil {
		return fmt.Errorf("events.Publish: marshal %s: %w", subject, err)
	}
	if _, err := p.js.Publish(ctx, subject, data); err != nil {
		return fmt.Errorf("events.Publish: publish %s: %w", subject, err)
	}
	return nil
}

// Close drains and closes the underlying NATS connection.
func (p *NATSPublisher) Close() {
	if p.nc != nil {
		p.nc.Drain() //nolint:errcheck // best-effort on shutdown
	}
}

// Subscribe creates (or resumes) a durable consumer on the ARK8DE stream and
// invokes handler for every message. Used by leaderboard-service to react to
// team/player events. Returns a stop function.
func (p *NATSPublisher) Subscribe(ctx context.Context, durable string, subjects []string, handler func(subject string, data []byte)) (func(), error) {
	cons, err := p.js.CreateOrUpdateConsumer(ctx, StreamName, jetstream.ConsumerConfig{
		Durable:        durable,
		FilterSubjects: subjects,
		AckPolicy:      jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("events.Subscribe: consumer %s: %w", durable, err)
	}

	cc, err := cons.Consume(func(msg jetstream.Msg) {
		handler(msg.Subject(), msg.Data())
		msg.Ack() //nolint:errcheck // redelivery on failed ack is acceptable
	})
	if err != nil {
		return nil, fmt.Errorf("events.Subscribe: consume %s: %w", durable, err)
	}
	return cc.Stop, nil
}
