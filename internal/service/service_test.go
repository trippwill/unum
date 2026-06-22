package service

import (
	"context"
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
