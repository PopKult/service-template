// Package grpcserver wraps a *grpc.Server with the standard interceptor
// chain (RED metrics, panic recovery, OTel tracing) and the standard
// health service already registered, per docs/microservice-standards.md
// §2.1 and §5.4.
package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"

	"github.com/PopKult/go-common/health"
	"github.com/PopKult/go-common/middleware/grpcmw"
)

// Server wraps a *grpc.Server with the standard interceptor chain and
// health service. Register this service's own gRPC services on
// GRPCServer() before calling Start.
type Server struct {
	grpcServer *grpc.Server
	health     *health.Server
	addr       string
	logger     *slog.Logger
}

// New builds a Server listening on addr, with go-common's standard
// interceptor chain and health service wired in.
func New(addr string, metrics *grpcmw.Metrics, logger *slog.Logger) *Server {
	h := health.NewServer()
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(grpcmw.TracingStatsHandler()),
		grpc.ChainUnaryInterceptor(
			grpcmw.RecoverUnaryServerInterceptor(logger),
			metrics.UnaryServerInterceptor(),
		),
	)
	h.RegisterOn(grpcServer)

	return &Server{grpcServer: grpcServer, health: h, addr: addr, logger: logger}
}

// GRPCServer exposes the underlying *grpc.Server so cmd/server can
// register this service's own gRPC services before Start.
func (s *Server) GRPCServer() *grpc.Server { return s.grpcServer }

// Health exposes the health server so internal/registry can register
// dependency checkers (DB, Kafka, ...) against it — see §5.4.
func (s *Server) Health() *health.Server { return s.health }

// Start listens and serves; it blocks until Stop closes the listener.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpcserver: listen on %s: %w", s.addr, err)
	}
	s.logger.Info("grpc server listening", slog.String("addr", s.addr))
	if err := s.grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("grpcserver: serve: %w", err)
	}
	return nil
}

// Stop implements go-common/shutdown.Server: it stops accepting new
// connections and waits for in-flight RPCs to finish, or ctx's deadline,
// whichever comes first.
func (s *Server) Stop(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.grpcServer.Stop()
	}
}
