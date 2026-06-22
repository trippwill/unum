package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/trippwill/unum/internal/config"
)

func TestStatusUsesConfig(t *testing.T) {
	cfg := config.Default()
	cfg.ServerName = "lab"
	cfg.Inference.Enabled = true
	cfg.Inference.Address = ":8770"
	cfg.Inference.BasePath = "/openai/v1/"
	cfg.Inference.DevInsecureHTTP = true
	cfg.Inference.ActiveProfile = "qwen3"

	got, err := New(cfg, "test-version").Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if got.ServerName != "lab" || got.Version != "test-version" || got.RuntimeBackend != "podman" {
		t.Fatalf("unexpected status: %+v", got)
	}
	if got.InferenceEndpoint != "http://localhost:8770/openai/v1" {
		t.Fatalf("InferenceEndpoint = %q", got.InferenceEndpoint)
	}
	if got.ActiveProfile != "qwen3" || got.Operations != "idle" {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestStatusOmitsDisabledInferenceEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.Inference.Enabled = false

	got, err := New(cfg, "test-version").Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.InferenceEndpoint != "" {
		t.Fatalf("InferenceEndpoint = %q", got.InferenceEndpoint)
	}
}

func TestListProfilesUsesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qwen.toml"), []byte(`id = "qwen"
name = "Qwen"

[runtime]
backend = "podman"

[image]
ref = "example"

[model]
path = "`+model+`"

[server]
kind = "openai-compatible"
host = "127.0.0.1"
port = 18080
health_path = "/health"

[container]
network = "host"
args = ["serve"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Storage.Profiles = dir
	got, err := New(cfg, "test-version").ListProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "qwen" || !got[0].Valid {
		t.Fatalf("profiles = %+v", got)
	}
}
