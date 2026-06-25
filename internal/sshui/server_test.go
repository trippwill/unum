package sshui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/profile"
	"github.com/trippwill/unum/internal/runtime/podman"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/version"
)

func TestDashboardViewShowsStatus(t *testing.T) {
	cfg := config.Default()
	cfg.ServerName = "lab"
	cfg.Inference.ActiveProfile = "qwen"
	cfg.Storage.State = t.TempDir()
	cfg.Storage.Profiles = t.TempDir()

	view := newDashboardModel(service.New(cfg, version.Version)).View()
	for _, want := range []string{"Unum Server", "Server:     lab", "Runtime:    podman", "Active:     qwen"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestDashboardCanSwitchToTokensAndCreateToken(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.State = t.TempDir()
	cfg.Storage.Profiles = t.TempDir()
	model := newDashboardModel(service.New(cfg, version.Version))

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	model = next.(dashboardModel)
	if !strings.Contains(model.View(), "Inference Tokens") {
		t.Fatalf("tokens page not shown:\n%s", model.View())
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	model = next.(dashboardModel)
	view := model.View()
	if !strings.Contains(view, "copy now: unum_sk_") || !strings.Contains(view, "active") {
		t.Fatalf("token create not shown:\n%s", view)
	}
}

func TestDashboardShowsContainerNameAndLoadsLogs(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.State = t.TempDir()
	cfg.Storage.Profiles = t.TempDir()
	modelPath := t.TempDir()
	writeProfile(t, filepath.Join(cfg.Storage.Profiles, "qwen.yaml"), "qwen", modelPath)
	runtime := &fakeRuntime{
		status: podman.ContainerStatus{
			ID:     "d76cd03fbf619f1cd2745587e89b752a8db35eb8476a6b9b02fe11b43a35aaea",
			Name:   "unum-qwen",
			State:  "running",
			Health: "unknown",
		},
		logs: []podman.LogLine{{Text: "llama server listening"}},
	}
	svc := service.New(cfg, version.Version, service.WithRuntimeBackend(runtime))
	if _, err := svc.StartProfile(context.Background(), "qwen"); err != nil {
		t.Fatal(err)
	}
	model := newDashboardModel(svc)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	model = next.(dashboardModel)
	view := model.View()
	if !strings.Contains(view, "unum-qwen") || !strings.Contains(view, "d76cd03fbf61") {
		t.Fatalf("instance display missing name or short id:\n%s", view)
	}
	if strings.Contains(view, "d76cd03fbf619f1cd2745587e89b752a8db35eb8476a6b9b02fe11b43a35aaea") {
		t.Fatalf("instance display leaked full id:\n%s", view)
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	model = next.(dashboardModel)
	view = model.View()
	if !strings.Contains(view, "Logs") || !strings.Contains(view, "llama server listening") || !strings.Contains(view, "loaded logs for unum-qwen") {
		t.Fatalf("logs not shown:\n%s", view)
	}
}

type fakeRuntime struct {
	status podman.ContainerStatus
	logs   []podman.LogLine
}

func (f *fakeRuntime) EnsureImage(context.Context, string) error { return nil }

func (f *fakeRuntime) Create(context.Context, profile.Profile) (podman.ContainerID, error) {
	return f.status.ID, nil
}

func (f *fakeRuntime) Start(context.Context, podman.ContainerID) error { return nil }

func (f *fakeRuntime) Stop(context.Context, podman.ContainerID) error { return nil }

func (f *fakeRuntime) Remove(context.Context, podman.ContainerID) error { return nil }

func (f *fakeRuntime) Inspect(context.Context, podman.ContainerID) (podman.ContainerStatus, error) {
	return f.status, nil
}

func (f *fakeRuntime) Logs(context.Context, podman.ContainerID, podman.LogOptions) (<-chan podman.LogLine, error) {
	ch := make(chan podman.LogLine, len(f.logs))
	for _, line := range f.logs {
		ch <- line
	}
	close(ch)
	return ch, nil
}

func writeProfile(t *testing.T, path, id, model string) {
	t.Helper()
	data := []byte(`services:
  ` + id + `:
    image: example
    network_mode: host
    volumes:
      - ` + model + `:/models:ro
    command: ["serve"]
x-unum:
  id: ` + id + `
  name: ` + id + `
  endpoints:
    openai:
      service: ` + id + `
      url: http://127.0.0.1:18080/v1
      health: /health
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
