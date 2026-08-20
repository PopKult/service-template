package grpcserver

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/PopKult/go-common/middleware/grpcmw"
	"github.com/prometheus/client_golang/prometheus"
)

func TestServer_startServeHealthCheckStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := grpcmw.NewMetrics(prometheus.NewRegistry())
	addr := "127.0.0.1:19191"
	srv := New(addr, metrics, logger)
	srv.Health().Register("dep", func(ctx context.Context) error { return nil })
	srv.Health().Refresh(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Stop(ctx)
		if err := <-errCh; err != nil {
			t.Errorf("Start returned error after Stop: %v", err)
		}
	}()

	conn := dialWithRetry(t, addr)
	defer func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close: %v", err)
		}
	}()

	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health Check: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.Status)
	}
}

func dialWithRetry(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			client := healthpb.NewHealthClient(conn)
			if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{}); err == nil {
				return conn
			}
			lastErr = err
			if closeErr := conn.Close(); closeErr != nil {
				t.Logf("conn.Close: %v", closeErr)
			}
		} else {
			lastErr = err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("could not connect to %s: %v", addr, lastErr)
	return nil
}
