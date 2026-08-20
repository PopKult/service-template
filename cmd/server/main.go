// Command server is this service's main entrypoint binary: loads config,
// builds the registry, and starts its entrypoints.
package main

import (
	"fmt"
	"log/slog"
	"os"

	commonconfig "github.com/PopKult/go-common/config"
	"github.com/PopKult/go-common/logging"
	"github.com/PopKult/go-common/shutdown"

	"github.com/PopKult/service-template/internal/config"
	"github.com/PopKult/service-template/internal/registry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := commonconfig.Load[config.Config]()
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	logger := logging.New(cfg.ServiceName, cfg.SlogLevel())

	ctx, cancel := shutdown.NotifyContext()
	defer cancel()

	reg, err := registry.New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	defer reg.Close()

	errCh := make(chan error, 2)
	go func() { errCh <- reg.GRPCServer.Start() }()
	go func() { errCh <- reg.MetricsServer.Start() }()

	logger.Info("service started",
		slog.Int("grpc_port", cfg.GRPCPort),
		slog.Int("metrics_port", cfg.MetricsPort),
	)

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		shutdown.Wait(ctx, cfg.ShutdownTimeout, reg.GRPCServer, reg.MetricsServer)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("main: %w", err)
		}
	}
	return nil
}
