// Package config defines this service's typed configuration, loaded from
// environment variables only (§6 in the repo root's
// docs/microservice-standards.md) — the same loading code runs in every
// environment; what differs is how the env vars themselves get set
// (docker-compose locally, ConfigMap/Secret in k8s).
package config

import (
	"log/slog"
	"strings"
	"time"
)

// Config is this service's fully typed configuration, loaded via
// github.com/PopKult/go-common/config.Load. cmd/server and
// cmd/outbox-relay both load the same struct and each use the subset of
// fields relevant to them.
type Config struct {
	ServiceName string `env:"SERVICE_NAME,required"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`

	GRPCPort    int `env:"GRPC_PORT" envDefault:"50051"`
	MetricsPort int `env:"METRICS_PORT" envDefault:"9090"`

	ShutdownTimeout     time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"20s"`
	HealthCheckInterval time.Duration `env:"HEALTH_CHECK_INTERVAL" envDefault:"10s"`

	PostgresHost     string `env:"POSTGRES_HOST,required"`
	PostgresPort     int    `env:"POSTGRES_PORT" envDefault:"5432"`
	PostgresUser     string `env:"POSTGRES_USER,required"`
	PostgresPassword string `env:"POSTGRES_PASSWORD,required"`
	PostgresDB       string `env:"POSTGRES_DB,required"`
	PostgresSSLMode  string `env:"POSTGRES_SSLMODE" envDefault:"disable"`

	KafkaBrokers []string `env:"KAFKA_BROKERS,required" envSeparator:","`
	OutboxTopic  string   `env:"OUTBOX_TOPIC,required"`

	OutboxRelayPollInterval time.Duration `env:"OUTBOX_RELAY_POLL_INTERVAL" envDefault:"2s"`
	OutboxRelayBatchSize    int           `env:"OUTBOX_RELAY_BATCH_SIZE" envDefault:"100"`

	OTelExporterEndpoint string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"otel-collector:4317"`
	OTelInsecure         bool    `env:"OTEL_EXPORTER_OTLP_INSECURE" envDefault:"true"`
	OTelSampleRatio      float64 `env:"OTEL_TRACES_SAMPLER_RATIO" envDefault:"1"`
}

// SlogLevel maps LogLevel ("debug"|"info"|"warn"|"error", case
// insensitive) to a slog.Level, defaulting to Info for an unrecognized
// value.
func (c Config) SlogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
