package registry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/PopKult/go-common/middleware/kafkamw"

	"github.com/PopKult/service-template/internal/config"
	"github.com/PopKult/service-template/internal/entrypoint/metricsserver"
	"github.com/PopKult/service-template/internal/repository/outbox"
	"github.com/PopKult/service-template/internal/repository/postgres"
)

// RelayRegistry is cmd/outbox-relay's own, smaller dependency graph:
// infra clients -> outbox repository -> relay. It's independent of
// Registry (cmd/server's) — the two binaries don't share an instance,
// only the go-common and repository packages, per §1.5.
type RelayRegistry struct {
	DB            *pgxpool.Pool
	Relay         *outbox.Relay
	Producer      *outbox.KafkaProducer
	MetricsServer *metricsserver.Server
	logger        *slog.Logger
}

// NewRelay constructs the relay's dependency graph.
func NewRelay(ctx context.Context, cfg config.Config, logger *slog.Logger) (*RelayRegistry, error) {
	promReg := prometheus.NewRegistry()
	promReg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	db, err := postgres.NewPool(ctx, postgres.PoolConfig{
		Host:     cfg.PostgresHost,
		Port:     cfg.PostgresPort,
		User:     cfg.PostgresUser,
		Password: cfg.PostgresPassword,
		Database: cfg.PostgresDB,
		SSLMode:  cfg.PostgresSSLMode,
	})
	if err != nil {
		return nil, fmt.Errorf("relay registry: connect postgres: %w", err)
	}

	producer := outbox.NewKafkaProducer(cfg.KafkaBrokers)
	metrics := kafkamw.NewMetrics(promReg)
	relay := outbox.NewRelay(db, producer, cfg.OutboxTopic, cfg.OutboxRelayBatchSize, metrics, logger)
	metricsServer := metricsserver.New(fmt.Sprintf(":%d", cfg.MetricsPort), promReg, logger)

	return &RelayRegistry{DB: db, Relay: relay, Producer: producer, MetricsServer: metricsServer, logger: logger}, nil
}

// Close releases infra resources that need explicit cleanup.
func (r *RelayRegistry) Close() {
	if err := r.Producer.Close(); err != nil {
		r.logger.Error("closing kafka producer", slog.Any("error", err))
	}
	r.DB.Close()
}
