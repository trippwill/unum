//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/inference"
	"github.com/trippwill/unum/internal/profile"
	"github.com/trippwill/unum/internal/runtime/podman"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/tokens"
)

func TestProfileTokenInferenceSmoke(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("upstream received Authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Storage.State = t.TempDir()
	cfg.Storage.Profiles = t.TempDir()
	writeProfile(t, filepath.Join(cfg.Storage.Profiles, "smoke.yaml"), upstream.URL+"/v1")

	svc := service.New(cfg, "integration-test", service.WithRuntimeBackend(fakeRuntime{}))
	handler := inference.NewHandler(cfg.Inference, svc, tokens.Store{Path: filepath.Join(cfg.Storage.State, "tokens", "inference-tokens.json")})

	if _, err := svc.StartProfile(context.Background(), "smoke"); err != nil {
		t.Fatal(err)
	}
	if err := svc.ActivateProfile(context.Background(), "smoke"); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateInferenceToken(context.Background(), "smoke")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+created.Raw)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
}

type fakeRuntime struct{}

func (fakeRuntime) EnsureImage(context.Context, string) error { return nil }

func (fakeRuntime) Create(context.Context, profile.Profile) (podman.ContainerID, error) {
	return "container-1", nil
}

func (fakeRuntime) Start(context.Context, podman.ContainerID) error { return nil }

func (fakeRuntime) Stop(context.Context, podman.ContainerID) error { return nil }

func (fakeRuntime) Remove(context.Context, podman.ContainerID) error { return nil }

func (fakeRuntime) Inspect(context.Context, podman.ContainerID) (podman.ContainerStatus, error) {
	return podman.ContainerStatus{
		ID:      "container-1",
		Name:    "unum-smoke",
		State:   "running",
		Health:  "healthy",
		Started: time.Unix(100, 0),
	}, nil
}

func (fakeRuntime) Logs(context.Context, podman.ContainerID, podman.LogOptions) (<-chan podman.LogLine, error) {
	ch := make(chan podman.LogLine)
	close(ch)
	return ch, nil
}

func writeProfile(t *testing.T, path, endpoint string) {
	t.Helper()
	data := []byte(`services:
  smoke:
    image: example/smoke:latest
    network_mode: host
    command: ["serve"]
x-unum:
  id: smoke
  name: Smoke
  endpoints:
    openai:
      service: smoke
      url: ` + endpoint + `
      health: /health
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
