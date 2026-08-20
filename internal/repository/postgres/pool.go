// Package postgres holds this service's own Postgres-backed repository
// implementations, plus the shared connection pool they're built from.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig is the subset of internal/config.Config that NewPool needs.
type PoolConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

func (c PoolConfig) dsn() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Database, c.SSLMode)
}

// NewPool opens a pgx connection pool and verifies connectivity with a
// ping before returning, so a bad connection fails construction in
// internal/registry (§1.5) rather than on the first query.
func NewPool(ctx context.Context, cfg PoolConfig) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}
