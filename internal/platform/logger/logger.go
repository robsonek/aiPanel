// Package logger provides structured logging (slog-based).
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a JSON logger configured for the given environment.
func New(env string) *slog.Logger {
	level := slog.LevelInfo
	if strings.EqualFold(env, "dev") {
		level = slog.LevelDebug
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(h)
}
