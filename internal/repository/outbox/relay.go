package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/PopKult/go-common/middleware/kafkamw"
)

// Relay polls unpublished outbox rows and publishes them to Kafka,
// giving at-least-once delivery: an event is only sent if the
// transaction that wrote it actually committed (§1.3). Consumers MUST be
// idempotent — polling can redeliver.
//
// RelayOnce holds each claimed row's lock for the duration of its
// publish call (SELECT ... FOR UPDATE SKIP LOCKED inside a transaction
// that isn't committed until the row is marked published). That's the
// simplest correct way to make multiple relay replicas safe to run
// concurrently. If publish latency ever becomes a bottleneck, split
// claiming (a fast UPDATE ... RETURNING) from publishing (outside any
// transaction) instead.
type Relay struct {
	pool      *pgxpool.Pool
	producer  Producer
	topic     string
	batchSize int
	metrics   *kafkamw.Metrics
	logger    *slog.Logger
}

// NewRelay constructs a Relay. metrics may be nil to skip RED
// instrumentation.
func NewRelay(pool *pgxpool.Pool, producer Producer, topic string, batchSize int, metrics *kafkamw.Metrics, logger *slog.Logger) *Relay {
	return &Relay{pool: pool, producer: producer, topic: topic, batchSize: batchSize, metrics: metrics, logger: logger}
}

type outboxRow struct {
	id          int64
	key         string
	payload     []byte
	traceparent string
}

// RelayOnce claims up to one batch of unpublished rows, publishes each to
// Kafka, and marks it published — all within one transaction per batch,
// so a concurrent relay instance skips rows this one already claimed. It
// returns how many rows were published.
func (r *Relay) RelayOnce(ctx context.Context) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("outbox: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_id, payload, COALESCE(traceparent, '')
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, r.batchSize)
	if err != nil {
		return 0, fmt.Errorf("outbox: query unpublished: %w", err)
	}

	var batch []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.id, &row.key, &row.payload, &row.traceparent); err != nil {
			rows.Close()
			return 0, fmt.Errorf("outbox: scan row: %w", err)
		}
		batch = append(batch, row)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("outbox: iterate rows: %w", err)
	}

	published := 0
	for _, row := range batch {
		start := time.Now()
		// Continue the trace of the request that wrote this row, so the
		// consumer's span hangs off it instead of a disconnected root.
		pubCtx, span := otel.Tracer("outbox-relay").Start(kafkamw.Restore(ctx, row.traceparent), "outbox publish "+r.topic,
			trace.WithSpanKind(trace.SpanKindProducer))
		headers := kafkamw.HeaderCarrier{}
		kafkamw.Inject(pubCtx, headers)

		pubErr := r.producer.Produce(ctx, Message{
			Topic:   r.topic,
			Key:     []byte(row.key),
			Value:   row.payload,
			Headers: headers,
		})
		if r.metrics != nil {
			r.metrics.ObserveProduce(r.topic, pubErr, time.Since(start))
		}
		if pubErr != nil {
			span.RecordError(pubErr)
			span.SetStatus(codes.Error, "publish failed")
			span.End()
			// Commit what's already been marked published in this batch
			// before returning — the rest stay unpublished (their locks
			// release on commit) and get retried on the next tick.
			if err := tx.Commit(ctx); err != nil {
				return published, fmt.Errorf("outbox: commit partial batch after publish failure: %w", err)
			}
			return published, fmt.Errorf("outbox: publish row %d: %w", row.id, pubErr)
		}

		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, row.id); err != nil {
			return published, fmt.Errorf("outbox: mark row %d published: %w", row.id, err)
		}
		span.End()
		published++
	}

	if err := tx.Commit(ctx); err != nil {
		return published, fmt.Errorf("outbox: commit batch: %w", err)
	}
	return published, nil
}

// Run polls at the given interval until ctx is done, logging (never
// swallowing) any error from RelayOnce so a transient DB/Kafka blip is
// visible without crashing the relay process.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := r.RelayOnce(ctx); err != nil {
				r.logger.ErrorContext(ctx, "outbox relay batch failed",
					slog.Any("error", err), slog.Int("published", n))
			}
		}
	}
}
