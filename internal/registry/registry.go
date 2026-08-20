// Package registry wires this service's dependency graph together in one
// place, per docs/microservice-standards.md §1.5: infra clients ->
// repositories -> use cases -> entrypoints. Plain, explicit, manual
// dependency injection — no reflection-based DI framework.
package registry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/PopKult/go-common/middleware/grpcmw"

	"github.com/PopKult/service-template/internal/config"
	"github.com/PopKult/service-template/internal/entrypoint/grpcserver"
	"github.com/PopKult/service-template/internal/entrypoint/metricsserver"
	"github.com/PopKult/service-template/internal/repository/postgres"
)

// Registry holds everything cmd/server needs to start and stop the
// service: entrypoints to serve, plus infra needing explicit cleanup. It
// is NOT a general-purpose service locator — use cases take their
// specific dependencies as constructor arguments, never the Registry
// itself.
type Registry struct {
	DB            *pgxpool.Pool
	GRPCServer    *grpcserver.Server
	MetricsServer *metricsserver.Server
}

// New constructs the registry, returning a wrapped error naming which
// dependency failed rather than panicking.
func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (*Registry, error) {
	promReg := prometheus.NewRegistry()
	promReg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// 1. infra clients
	db, err := postgres.NewPool(ctx, postgres.PoolConfig{
		Host:     cfg.PostgresHost,
		Port:     cfg.PostgresPort,
		User:     cfg.PostgresUser,
		Password: cfg.PostgresPassword,
		Database: cfg.PostgresDB,
		SSLMode:  cfg.PostgresSSLMode,
	})
	if err != nil {
		return nil, fmt.Errorf("registry: connect postgres: %w", err)
	}

	// 2. repositories
	// Add this service's repositories here as use cases need them, e.g.:
	//   widgetRepo := postgres.NewWidgetRepository(db)

	// 3. use cases
	// Add this service's use cases here, e.g.:
	//   createWidget := createwidget.New(widgetRepo, outboxRepo)

	// 4. entrypoints
	grpcServer := grpcserver.New(fmt.Sprintf(":%d", cfg.GRPCPort), grpcmw.NewMetrics(promReg), logger)
	grpcServer.Health().Register("postgres", func(ctx context.Context) error {
		return db.Ping(ctx)
	})
	metricsServer := metricsserver.New(fmt.Sprintf(":%d", cfg.MetricsPort), promReg, logger)

	return &Registry{DB: db, GRPCServer: grpcServer, MetricsServer: metricsServer}, nil
}

// Close releases infra resources that need explicit cleanup. Call it
// after cmd/server's servers have all stopped.
func (r *Registry) Close() {
	r.DB.Close()
}
