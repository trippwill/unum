package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	want := Default()
	want.ServerName = "lab"
	want.Storage.State = t.TempDir()

	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "unumd.toml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.ServerName != "lab" || got.Runtime.Backend != "podman" || got.Inference.BasePath != "/openai/v1" {
		t.Fatalf("unexpected config: %+v", got)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unumd.toml")
	if err := os.WriteFile(path, []byte("server_nmae = \"typo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted an unknown field")
	}
}

func TestLoadStartsFromDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unumd.toml")
	if err := os.WriteFile(path, []byte("server_name = \"lab\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "lab" || cfg.Runtime.Backend != "podman" {
		t.Fatalf("defaults were not applied: %+v", cfg)
	}
}
