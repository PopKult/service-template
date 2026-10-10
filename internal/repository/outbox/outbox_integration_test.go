//go:build integration

// Integration tests for the parts unit tests with mocks can't verify:
// real SQL against a real Postgres, per docs/microservice-standards.md
// §4. Run with: go test -tags=integration ./internal/repository/outbox/...
package outbox

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"

	"github.com/PopKult/go-common/middleware/kafkamw"
)

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)

	for _, path := range migrationPaths(t) {
		migration, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}

	return pool
}

// migrationPaths returns every migrations/*.up.sql in order, found relative
// to this test file, independent of the working directory `go test` runs from.
func migrationPaths(t *testing.T) []string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	paths, err := filepath.Glob(filepath.Join(thisFile, "..", "..", "..", "..", "migrations", "*.up.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("find migrations: %v (found %d)", err, len(paths))
	}
	sort.Strings(paths)
	return paths
}

func TestWriteAndRelay_endToEnd(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := Write(ctx, tx, Event{
		AggregateType: "widget",
		AggregateID:   "widget-1",
		EventType:     "widget.created",
		Payload:       []byte("proto-bytes"),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	ctrl := gomock.NewController(t)
	mockProducer := NewMockProducer(ctrl)
	mockProducer.EXPECT().
		Produce(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, msg Message) error {
			if msg.Topic != "widget.events" {
				t.Errorf("Topic = %q, want widget.events", msg.Topic)
			}
			if string(msg.Key) != "widget-1" {
				t.Errorf("Key = %q, want widget-1", msg.Key)
			}
			if string(msg.Value) != "proto-bytes" {
				t.Errorf("Value = %q, want proto-bytes", msg.Value)
			}
			return nil
		}).
		Times(1)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	relay := NewRelay(pool, mockProducer, "widget.events", 10, nil, logger)

	published, err := relay.RelayOnce(ctx)
	if err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1", published)
	}

	// A second pass must find nothing left to publish.
	published, err = relay.RelayOnce(ctx)
	if err != nil {
		t.Fatalf("RelayOnce (2nd pass): %v", err)
	}
	if published != 0 {
		t.Fatalf("published on 2nd pass = %d, want 0 (row already marked published)", published)
	}
}

func TestRelayOnce_publishFailure_leavesRowUnpublished(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := Write(ctx, tx, Event{
		AggregateType: "widget",
		AggregateID:   "widget-2",
		EventType:     "widget.created",
		Payload:       []byte("proto-bytes"),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	ctrl := gomock.NewController(t)
	mockProducer := NewMockProducer(ctrl)
	mockProducer.EXPECT().
		Produce(gomock.Any(), gomock.Any()).
		Return(context.DeadlineExceeded).
		Times(1)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	relay := NewRelay(pool, mockProducer, "widget.events", 10, nil, logger)

	if _, err := relay.RelayOnce(ctx); err == nil {
		t.Fatal("expected RelayOnce to return the publish error")
	}

	var publishedAt *time.Time
	if err := pool.QueryRow(ctx, `SELECT published_at FROM outbox_events WHERE aggregate_id = $1`, "widget-2").Scan(&publishedAt); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if publishedAt != nil {
		t.Errorf("published_at = %v, want NULL after a failed publish", publishedAt)
	}
}

// A row written inside a traced request must be published as a
// continuation of that trace, even though the relay runs in its own loop.
func TestWriteAndRelay_propagatesTraceContext(t *testing.T) {
	pool := newTestPool(t)

	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	reqCtx, reqSpan := tp.Tracer("test").Start(context.Background(), "request")
	wantTraceID := reqSpan.SpanContext().TraceID()

	tx, err := pool.Begin(reqCtx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := Write(reqCtx, tx, Event{
		AggregateType: "widget", AggregateID: "widget-3", EventType: "widget.created", Payload: []byte("p"),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tx.Commit(reqCtx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	reqSpan.End()

	ctrl := gomock.NewController(t)
	mockProducer := NewMockProducer(ctrl)
	mockProducer.EXPECT().
		Produce(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, msg Message) error {
			got := trace.SpanContextFromContext(kafkamw.Extract(context.Background(), kafkamw.HeaderCarrier(msg.Headers)))
			if got.TraceID() != wantTraceID {
				t.Errorf("published trace id = %s, want %s (header %q)", got.TraceID(), wantTraceID, msg.Headers["traceparent"])
			}
			return nil
		}).
		Times(1)

	relay := NewRelay(pool, mockProducer, "widget.events", 10, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	// The relay's own loop context has no span; the trace must come from the row.
	if _, err := relay.RelayOnce(context.Background()); err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
}
