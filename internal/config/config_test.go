package config

import (
	"log/slog"
	"testing"
)

func TestConfig_SlogLevel(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want slog.Level
	}{
		{name: "debug", in: "debug", want: slog.LevelDebug},
		{name: "warn", in: "warn", want: slog.LevelWarn},
		{name: "error", in: "error", want: slog.LevelError},
		{name: "info", in: "info", want: slog.LevelInfo},
		{name: "uppercase is case insensitive", in: "DEBUG", want: slog.LevelDebug},
		{name: "unrecognized defaults to info", in: "verbose", want: slog.LevelInfo},
		{name: "empty defaults to info", in: "", want: slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{LogLevel: tt.in}
			if got := cfg.SlogLevel(); got != tt.want {
				t.Errorf("SlogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}
