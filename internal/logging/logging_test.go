package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNew_LevelFiltersLowerLevels(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, slog.LevelWarn, "text")
	log.Info("quiet-message")
	log.Warn("loud-message")

	out := buf.String()
	if strings.Contains(out, "quiet-message") {
		t.Errorf("info record leaked through at warn level: %q", out)
	}
	if !strings.Contains(out, "loud-message") {
		t.Errorf("warn record missing: %q", out)
	}
}

func TestNew_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, "json").Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if rec["msg"] != "hello" || rec["k"] != "v" || rec["level"] != "INFO" {
		t.Errorf("unexpected record: %v", rec)
	}
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, slog.LevelInfo, "text").Info("hello", "k", "v")

	out := buf.String()
	if !strings.Contains(out, "msg=hello") || !strings.Contains(out, "k=v") {
		t.Errorf("unexpected text output: %q", out)
	}
}
