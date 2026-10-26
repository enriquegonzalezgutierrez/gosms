package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaultsWhenMissing(t *testing.T) {
	cfg := loadConfigFrom(filepath.Join(t.TempDir(), "missing.json"))
	if cfg.RomsDir != "roms" {
		t.Fatalf("default ROM directory = %q, want roms", cfg.RomsDir)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gosms_config.json")
	want := Config{RomsDir: "games"}
	if err := saveConfigTo(path, want); err != nil {
		t.Fatalf("saveConfigTo() error = %v", err)
	}
	if got := loadConfigFrom(path); got != want {
		t.Fatalf("loadConfigFrom() = %#v, want %#v", got, want)
	}
}

func TestLoadConfigDefaultsOnInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFrom(path); got.RomsDir != "roms" {
		t.Fatalf("loadConfigFrom(invalid) = %#v, want default ROM directory", got)
	}
}
