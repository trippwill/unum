package sshui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	charmssh "github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/sshkeys"
)

func Serve(ctx context.Context, cfg config.Config, svc *service.Service) error {
	if !cfg.SSHTUI.Enabled {
		return fmt.Errorf("ssh tui is disabled")
	}
	server, err := NewServer(cfg, svc)
	if err != nil {
		return err
	}

	errc := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, charmssh.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, charmssh.ErrServerClosed) {
			return fmt.Errorf("stop ssh tui: %w", err)
		}
		return nil
	}
}

func NewServer(cfg config.Config, svc *service.Service) (*charmssh.Server, error) {
	keys := sshkeys.Store{Path: filepath.Join(cfg.Storage.State, "ssh", "authorized-clients.json")}
	return wish.NewServer(
		wish.WithAddress(cfg.SSHTUI.Address),
		wish.WithHostKeyPath(cfg.SSHTUI.HostKey),
		wish.WithPublicKeyAuth(func(_ charmssh.Context, key charmssh.PublicKey) bool {
			_, ok, err := keys.Authorize(key)
			if err != nil {
				log.Printf("ssh auth failed: %v", err)
			}
			return ok && err == nil
		}),
		wish.WithMiddleware(
			bubbletea.Middleware(func(_ charmssh.Session) (tea.Model, []tea.ProgramOption) {
				return newDashboardModel(svc), []tea.ProgramOption{tea.WithAltScreen()}
			}),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
}

type dashboardModel struct {
	svc    *service.Service
	status service.Status
	err    error
	width  int
	height int
}

func newDashboardModel(svc *service.Service) dashboardModel {
	status, err := svc.Status(context.Background())
	return dashboardModel{svc: svc, status: status, err: err}
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("Unum Server")
	if m.err != nil {
		return title + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error()) + "\n"
	}
	rows := []string{
		"Server:     " + m.status.ServerName,
		"Version:    " + m.status.Version,
		"Runtime:    " + m.status.RuntimeBackend,
		"SSH:        " + m.status.SSHAddress,
		"Inference:  " + m.status.InferenceEndpoint,
		"Active:     " + emptyDash(m.status.ActiveProfile),
		"Operations: " + m.status.Operations,
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("q quits")
	return title + "\n" + strings.Repeat("─", len("Unum Server")) + "\n\n" + strings.Join(rows, "\n") + "\n\n" + help + "\n"
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
