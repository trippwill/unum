package sshui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/version"
)

func TestDashboardViewShowsStatus(t *testing.T) {
	cfg := config.Default()
	cfg.ServerName = "lab"
	cfg.Inference.ActiveProfile = "qwen"

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
