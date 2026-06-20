package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dphilli/meshtastic-ham-bridge/internal/config"
)

func TestRoundTrip(t *testing.T) {
	cfg := config.Template()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	if err := cfg.SaveTo(path); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Bridge) != len(cfg.Bridge) {
		t.Fatalf("got %d bridges, want %d", len(loaded.Bridge), len(cfg.Bridge))
	}
	if loaded.Bridge[0].Name != "vhf-local" {
		t.Fatalf("got name %q", loaded.Bridge[0].Name)
	}
}

func TestMissingFileReturnsEmpty(t *testing.T) {
	os.Unsetenv("MESHTASTIC_HAM_CONFIG")
	// Point at a non-existent file via env var
	os.Setenv("MESHTASTIC_HAM_CONFIG", "/nonexistent/config.toml")
	defer os.Unsetenv("MESHTASTIC_HAM_CONFIG")

	_, err := config.Load("")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write config with minimal fields
	if err := os.WriteFile(path, []byte(`
[[bridge]]
name = "test"

[bridge.mesh]
type = "meshtastic"

[bridge.ham]
type = "direwolf"
`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Bridge[0]
	if b.Ham.Host != "127.0.0.1" {
		t.Fatalf("default host: got %q", b.Ham.Host)
	}
	if b.Ham.Port != 8001 {
		t.Fatalf("default port: got %d", b.Ham.Port)
	}
	if b.Mesh.BaudRate != 115200 {
		t.Fatalf("default baud: got %d", b.Mesh.BaudRate)
	}
}

func TestDefaultPathIsAbsolute(t *testing.T) {
	p := config.DefaultPath()
	if !filepath.IsAbs(p) {
		t.Fatalf("default path not absolute: %s", p)
	}
}
