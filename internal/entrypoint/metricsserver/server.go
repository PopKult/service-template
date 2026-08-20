// Package metricsserver exposes the Prometheus registry populated by
// go-common's RED middleware on a plain HTTP /metrics endpoint, per
// docs/microservice-standards.md §5.1.
package metricsserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server serves GET /metrics for Prometheus to scrape.
type Server struct {
	httpServer *http.Server
	addr       string
	logger     *slog.Logger
}

// New builds a Server listening on addr, exposing reg's metrics.
func New(addr string, reg *prometheus.Registry, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	return &Server{
		httpServer: &http.Server{Addr: addr, Handler: mux},
		addr:       addr,
		logger:     logger,
	}
}

// Start listens and serves; it blocks until Stop shuts the server down.
func (s *Server) Start() error {
	s.logger.Info("metrics server listening", slog.String("addr", s.addr))
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("metricsserver: serve: %w", err)
	}
	return nil
}

// Stop implements go-common/shutdown.Server.
func (s *Server) Stop(ctx context.Context) {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.ErrorContext(ctx, "metrics server shutdown error", slog.Any("error", err))
	}
}
