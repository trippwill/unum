package inference

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/tokens"
)

func TestHandlerProxiesAuthorizedRequestToActiveInstance(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("upstream path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("upstream received Authorization header %q", got)
		}
		w.WriteHeader(http.StatusTeapot)
	}))
	defer upstream.Close()
	store := tokens.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}
	created, err := store.Create("editor")
	if err != nil {
		t.Fatal(err)
	}
	control := fakeControl{
		status: service.Status{ActiveProfile: "qwen"},
		instances: []service.InstanceSummary{{
			ProfileID: "qwen",
			State:     "running",
			Endpoint:  upstream.URL,
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+created.Raw)
	rec := httptest.NewRecorder()

	NewHandler(config.Default().Inference, control, store).ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
}

func TestHandlerRejectsMissingToken(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(config.Default().Inference, fakeControl{}, tokens.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestHandlerReturns503WithoutActiveProfile(t *testing.T) {
	store := tokens.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}
	created, err := store.Create("editor")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+created.Raw)
	rec := httptest.NewRecorder()

	NewHandler(config.Default().Inference, fakeControl{}, store).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestServeRejectsInsecureNonLoopback(t *testing.T) {
	cfg := config.Default().Inference
	cfg.Address = "0.0.0.0:8770"
	if err := Serve(context.Background(), cfg, fakeControl{}, tokens.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}); err == nil {
		t.Fatal("Serve accepted insecure non-loopback address")
	}
}

func TestServeRejectsInsecureWildcardAddress(t *testing.T) {
	cfg := config.Default().Inference
	cfg.Address = ":8770"
	if err := Serve(context.Background(), cfg, fakeControl{}, tokens.Store{Path: filepath.Join(t.TempDir(), "tokens.json")}); err == nil {
		t.Fatal("Serve accepted insecure wildcard address")
	}
}

type fakeControl struct {
	status    service.Status
	instances []service.InstanceSummary
}

func (f fakeControl) Status(context.Context) (service.Status, error) {
	return f.status, nil
}

func (f fakeControl) ListInstances(context.Context) ([]service.InstanceSummary, error) {
	return f.instances, nil
}
