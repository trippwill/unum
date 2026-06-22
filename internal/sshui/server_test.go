package sshui

import (
	"strings"
	"testing"

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
