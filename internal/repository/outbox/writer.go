// Package outbox implements the transactional outbox: use cases MUST NOT
// publish to Kafka directly (§1.3). Write inserts an outbox row in the
// same Postgres transaction as the state change it describes; Relay (in
// this package, running as cmd/outbox-relay) polls unpublished rows and
// publishes them, giving at-least-once delivery.
package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Event is one row to write to the outbox in the same transaction as the
// state change it describes.
type Event struct {
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte // Protobuf-encoded, per docs/microservice-standards.md §2.2
}

// Write inserts ev into the outbox table using tx, so it commits or
// rolls back atomically with whatever other state change tx contains.
// Call this from a repository method invoked by a use case inside that
// use case's own transaction.
func Write(ctx context.Context, tx pgx.Tx, ev Event) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4)`,
		ev.AggregateType, ev.AggregateID, ev.EventType, ev.Payload,
	)
	if err != nil {
		return fmt.Errorf("outbox: write event: %w", err)
	}
	return nil
}
