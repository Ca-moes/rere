package cli

import (
	"io"
	"log/slog"
)

// loggerFor builds a logger whose level is Debug when verbose, else Info. The
// pipeline installs it as the default so the slog.Debug calls in adapter and
// discover (skipped scans, unparseable files) emit only under --verbose.
func loggerFor(verbose bool, w io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
