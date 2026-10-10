// Command outbox-relay is a separate binary that polls this service's
// outbox table and publishes unpublished rows to Kafka, per
// docs/microservice-standards.md §1.3.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	commonconfig "github.com/PopKult/go-common/config"
	"github.com/PopKult/go-common/logging"
	"github.com/PopKult/go-common/shutdown"
	"github.com/PopKult/go-common/telemetry"

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
	logger := logging.New(cfg.ServiceName+"-outbox-relay", cfg.SlogLevel())

	ctx, cancel := shutdown.NotifyContext()
	defer cancel()

	// Install the OTel SDK + W3C propagator; without it tracing middleware
	// is a no-op. Flush pending spans last, after the servers drain.
	shutdownTelemetry, err := telemetry.Init(ctx, telemetry.Config{
		ServiceName: cfg.ServiceName + "-outbox-relay",
		Endpoint:    cfg.OTelExporterEndpoint,
		Insecure:    cfg.OTelInsecure,
		SampleRatio: cfg.OTelSampleRatio,
	})
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	defer func() {
		if err := shutdownTelemetry(context.Background()); err != nil {
			logger.Error("flushing telemetry", slog.Any("error", err))
		}
	}()

	reg, err := registry.NewRelay(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("main: %w", err)
	}
	defer reg.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- reg.MetricsServer.Start() }()
	go reg.Relay.Run(ctx, cfg.OutboxRelayPollInterval)

	logger.Info("outbox relay started", slog.Int("metrics_port", cfg.MetricsPort))

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining")
		shutdown.Wait(ctx, cfg.ShutdownTimeout, reg.MetricsServer)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("main: %w", err)
		}
	}
	return nil
}
