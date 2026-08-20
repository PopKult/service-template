package metricsserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

func TestServer_startServeMetricsStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())

	addr := "127.0.0.1:19192"
	srv := New(addr, reg, logger)

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

	resp := getWithRetry(t, "http://"+addr+"/metrics")
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("resp.Body.Close: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func getWithRetry(t *testing.T, url string) *http.Response {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			return resp
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("could not GET %s: %v", url, lastErr)
	return nil
}
