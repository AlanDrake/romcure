// Package applog provides structured logging to a file in the app data
// dir (spec §2.1's <app_data_root>), plus the app-data-dir resolution
// other packages (store, transcode cache, ...) will also need.
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// AppDataDir returns the app's data directory (<user config dir>/romcure),
// creating it if necessary. This is the <app_data_root> referenced
// throughout spec §2.1 (library.db, transcode-cache/, scratch/ all live
// under it). os.UserConfigDir() already resolves the right per-OS
// location (%AppData% on Windows, ~/Library/Application Support on macOS,
// $XDG_CONFIG_HOME or ~/.config on Linux), so no per-OS code is needed.
func AppDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	dir := filepath.Join(base, "romcure")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create app data dir %s: %w", dir, err)
	}
	return dir, nil
}

// NewWithWriter builds a structured (JSON) logger writing to w at the
// given minimum level. Exposed directly so callers (and tests) that
// already have a destination don't need a real file.
func NewWithWriter(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

// NewInDir opens (creating or appending to) romcure.log inside dir and
// returns a logger writing structured JSON records to it, plus a closer
// the caller must call before the app exits. Pass the result of
// AppDataDir() in production; tests can pass any directory.
func NewInDir(dir string) (*slog.Logger, func() error, error) {
	path := filepath.Join(dir, "romcure.log")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %s: %w", path, err)
	}
	return NewWithWriter(f, slog.LevelInfo), f.Close, nil
}
