package applog

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppDataDirEndsInTheProductNameAndExists(t *testing.T) {
	dir, err := AppDataDir()
	if err != nil {
		t.Fatalf("AppDataDir: %v", err)
	}
	if filepath.Base(dir) != "romcure" {
		t.Errorf("AppDataDir() = %s, want a path ending in \"romcure\"", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(%s): %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s exists but is not a directory", dir)
	}
}

func lastLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no output written")
	}
	lines := strings.Split(line, "\n")
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("parse log line %q as JSON: %v", lines[len(lines)-1], err)
	}
	return record
}

func TestNewWithWriterLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		minLevel    slog.Level
		logAt       slog.Level
		wantWritten bool
	}{
		{"info logger, info message -> written", slog.LevelInfo, slog.LevelInfo, true},
		{"info logger, debug message -> filtered out", slog.LevelInfo, slog.LevelDebug, false},
		{"debug logger, debug message -> written", slog.LevelDebug, slog.LevelDebug, true},
		{"info logger, error message -> written", slog.LevelInfo, slog.LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewWithWriter(&buf, tt.minLevel)
			logger.Log(context.Background(), tt.logAt, "test message", "key", "value")

			got := buf.Len() > 0
			if got != tt.wantWritten {
				t.Errorf("wrote output = %v, want %v (buf: %q)", got, tt.wantWritten, buf.String())
			}
		})
	}
}

func TestNewWithWriterStructure(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, slog.LevelInfo)
	logger.Info("sync completed", "profile_id", "retroarch", "games_linked", 42)

	record := lastLogLine(t, &buf)
	if record["msg"] != "sync completed" {
		t.Errorf("msg = %v, want %q", record["msg"], "sync completed")
	}
	if record["profile_id"] != "retroarch" {
		t.Errorf("profile_id = %v, want %q", record["profile_id"], "retroarch")
	}
	if record["games_linked"] != float64(42) { // JSON numbers decode as float64
		t.Errorf("games_linked = %v, want 42", record["games_linked"])
	}
	if _, ok := record["time"]; !ok {
		t.Error("record has no \"time\" field")
	}
	if record["level"] != "INFO" {
		t.Errorf("level = %v, want %q", record["level"], "INFO")
	}
}

func TestNewInDir(t *testing.T) {
	dir := t.TempDir()

	logger, closeFn, err := NewInDir(dir)
	if err != nil {
		t.Fatalf("NewInDir: %v", err)
	}
	logger.Info("first line")
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "romcure.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "first line") {
		t.Errorf("log file content = %q, want it to contain %q", data, "first line")
	}

	// Re-opening must append, not truncate.
	logger2, closeFn2, err := NewInDir(dir)
	if err != nil {
		t.Fatalf("second NewInDir: %v", err)
	}
	logger2.Info("second line")
	if err := closeFn2(); err != nil {
		t.Fatalf("close: %v", err)
	}

	data, err = os.ReadFile(filepath.Join(dir, "romcure.log"))
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "first line") || !strings.Contains(string(data), "second line") {
		t.Errorf("log file content = %q, want both lines present (append, not truncate)", data)
	}
}
