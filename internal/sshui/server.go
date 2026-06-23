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
	svc           *service.Service
	page          page
	status        service.Status
	profiles      []service.ProfileSummary
	profileIndex  int
	instances     []service.InstanceSummary
	instanceIndex int
	operations    []service.OperationSummary
	tokens        []service.InferenceTokenSummary
	tokenIndex    int
	logs          []service.LogLine
	message       string
	err           error
	width         int
	height        int
}

type page int

const (
	pageDashboard page = iota
	pageProfiles
	pageInstances
	pageLogs
	pageOperations
	pageTokens
)

func newDashboardModel(svc *service.Service) dashboardModel {
	return dashboardModel{svc: svc}.refresh()
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
		case "1":
			m.page = pageDashboard
		case "2":
			m.page = pageProfiles
		case "3":
			m.page = pageInstances
		case "4":
			m.page = pageLogs
		case "5":
			m.page = pageOperations
		case "6":
			m.page = pageTokens
		case "r":
			m = m.refresh()
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "a":
			m = m.activateProfile()
		case "s":
			m = m.startProfile()
		case "x":
			m = m.stopOrRevoke()
		case "l":
			m = m.loadLogs()
		case "c":
			m = m.createToken()
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Render("Unum Server")
	body := m.viewDashboard()
	switch m.page {
	case pageProfiles:
		body = m.viewProfiles()
	case pageInstances:
		body = m.viewInstances()
	case pageLogs:
		body = m.viewLogs()
	case pageOperations:
		body = m.viewOperations()
	case pageTokens:
		body = m.viewTokens()
	}
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("1 dashboard  2 profiles  3 instances  4 logs  5 ops  6 tokens  r refresh  q quit")
	message := ""
	if m.message != "" {
		message = "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(m.message)
	}
	if m.err != nil {
		message = "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.err.Error())
	}
	return title + "\n" + strings.Repeat("─", len("Unum Server")) + "\n\n" + body + message + "\n\n" + help + "\n"
}

func (m dashboardModel) refresh() dashboardModel {
	ctx := context.Background()
	m.status, m.err = m.svc.Status(ctx)
	if m.err != nil {
		return m
	}
	m.profiles, m.err = m.svc.ListProfiles(ctx)
	if m.err != nil {
		return m
	}
	m.instances, m.err = m.svc.ListInstances(ctx)
	if m.err != nil {
		return m
	}
	m.operations, m.err = m.svc.ListOperations(ctx)
	if m.err != nil {
		return m
	}
	m.tokens, m.err = m.svc.ListInferenceTokens(ctx)
	if m.err != nil {
		return m
	}
	m.profileIndex = clamp(m.profileIndex, len(m.profiles))
	m.instanceIndex = clamp(m.instanceIndex, len(m.instances))
	m.tokenIndex = clamp(m.tokenIndex, len(m.tokens))
	return m
}

func (m *dashboardModel) move(delta int) {
	switch m.page {
	case pageProfiles:
		m.profileIndex = moveIndex(m.profileIndex, delta, len(m.profiles))
	case pageInstances, pageLogs:
		m.instanceIndex = moveIndex(m.instanceIndex, delta, len(m.instances))
	case pageTokens:
		m.tokenIndex = moveIndex(m.tokenIndex, delta, len(m.tokens))
	}
}

func (m dashboardModel) activateProfile() dashboardModel {
	if m.page != pageProfiles || len(m.profiles) == 0 {
		return m
	}
	id := m.profiles[m.profileIndex].ID
	if err := m.svc.ActivateProfile(context.Background(), id); err != nil {
		m.message = err.Error()
		return m
	}
	m.message = "activated " + id
	return m.refresh()
}

func (m dashboardModel) startProfile() dashboardModel {
	if m.page != pageProfiles || len(m.profiles) == 0 {
		return m
	}
	id := m.profiles[m.profileIndex].ID
	op, err := m.svc.StartProfile(context.Background(), id)
	if err != nil {
		m.message = op.State + " " + op.Phase + ": " + err.Error()
		return m.refresh()
	}
	m.message = op.State + " " + op.Phase
	return m.refresh()
}

func (m dashboardModel) stopOrRevoke() dashboardModel {
	switch m.page {
	case pageProfiles:
		if len(m.profiles) == 0 {
			return m
		}
		id := m.profiles[m.profileIndex].ID
		op, err := m.svc.StopProfile(context.Background(), id)
		if err != nil {
			m.message = op.State + " " + op.Phase + ": " + err.Error()
			return m.refresh()
		}
		m.message = op.State + " " + op.Phase
		return m.refresh()
	case pageTokens:
		if len(m.tokens) == 0 {
			return m
		}
		id := m.tokens[m.tokenIndex].ID
		if err := m.svc.RevokeInferenceToken(context.Background(), id); err != nil {
			m.message = err.Error()
			return m
		}
		m.message = "revoked " + id
		return m.refresh()
	default:
		return m
	}
}

func (m dashboardModel) createToken() dashboardModel {
	if m.page != pageTokens {
		return m
	}
	name := "tui-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	created, err := m.svc.CreateInferenceToken(context.Background(), name)
	if err != nil {
		m.message = err.Error()
		return m
	}
	m.message = "copy now: " + created.Raw
	return m.refresh()
}

func (m dashboardModel) loadLogs() dashboardModel {
	if len(m.instances) == 0 {
		m.message = "no instances"
		return m
	}
	id := m.instances[m.instanceIndex].ID
	logs, err := m.svc.TailLogs(context.Background(), id, 100)
	if err != nil {
		m.message = err.Error()
		return m
	}
	m.logs = logs
	m.page = pageLogs
	m.message = "loaded logs for " + instanceLabel(m.instances[m.instanceIndex])
	return m
}

func (m dashboardModel) viewDashboard() string {
	rows := []string{
		"Server:     " + m.status.ServerName,
		"Version:    " + m.status.Version,
		"Runtime:    " + m.status.RuntimeBackend,
		"SSH:        " + m.status.SSHAddress,
		"Inference:  " + m.status.InferenceEndpoint,
		"Active:     " + emptyDash(m.status.ActiveProfile),
		"Operations: " + m.status.Operations,
	}
	return strings.Join(rows, "\n")
}

func (m dashboardModel) viewProfiles() string {
	if len(m.profiles) == 0 {
		return "Profiles\n\n(no profiles)\n\ns start  a activate  x stop"
	}
	rows := []string{"Profiles", ""}
	for i, p := range m.profiles {
		marker := " "
		if i == m.profileIndex {
			marker = ">"
		}
		valid := "invalid"
		if p.Valid {
			valid = "valid"
		}
		rows = append(rows, fmt.Sprintf("%s %s  %s  %s  %s", marker, p.ID, valid, p.State, p.Reason))
	}
	return strings.Join(rows, "\n") + "\n\nj/k select  s start  a activate  x stop"
}

func (m dashboardModel) viewInstances() string {
	if len(m.instances) == 0 {
		return "Instances\n\n(no instances)\n\nl load logs"
	}
	rows := []string{"Instances", ""}
	for i, instance := range m.instances {
		marker := " "
		if i == m.instanceIndex {
			marker = ">"
		}
		rows = append(rows, fmt.Sprintf("%s %s  %s  %s  %s  %s  %s", marker, instanceLabel(instance), shortID(instance.ID), instance.ProfileID, instance.State, instance.Health, instance.StartedAt))
	}
	return strings.Join(rows, "\n") + "\n\nj/k select  l load logs"
}

func (m dashboardModel) viewLogs() string {
	if len(m.logs) == 0 {
		return "Logs\n\n(no logs loaded)\n\nSelect an instance, press l."
	}
	rows := []string{"Logs", ""}
	for _, line := range m.logs {
		if line.Err != nil {
			rows = append(rows, "error: "+line.Err.Error())
			continue
		}
		rows = append(rows, line.Text)
	}
	return strings.Join(rows, "\n")
}

func (m dashboardModel) viewOperations() string {
	if len(m.operations) == 0 {
		return "Operations\n\n(no operations)"
	}
	rows := []string{"Operations", ""}
	for _, op := range m.operations {
		rows = append(rows, fmt.Sprintf("%s  %s  %s  %s  %s", op.ID, op.Target, op.State, op.Phase, op.Message))
	}
	return strings.Join(rows, "\n")
}

func (m dashboardModel) viewTokens() string {
	rows := []string{"Inference Tokens", ""}
	if len(m.tokens) == 0 {
		rows = append(rows, "(no tokens)")
	} else {
		for i, token := range m.tokens {
			marker := " "
			if i == m.tokenIndex {
				marker = ">"
			}
			status := "active"
			if token.Revoked {
				status = "revoked"
			}
			rows = append(rows, fmt.Sprintf("%s %s  %s  %s  %s", marker, token.ID, token.Name, token.Prefix, status))
		}
	}
	return strings.Join(rows, "\n") + "\n\nc create  x revoke"
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func instanceLabel(instance service.InstanceSummary) string {
	if instance.Name != "" {
		return instance.Name
	}
	if instance.ProfileID != "" {
		return instance.ProfileID
	}
	return shortID(instance.ID)
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func clamp(i, length int) int {
	if length == 0 || i < 0 {
		return 0
	}
	if i >= length {
		return length - 1
	}
	return i
}

func moveIndex(i, delta, length int) int {
	if length == 0 {
		return 0
	}
	i += delta
	if i < 0 {
		return length - 1
	}
	if i >= length {
		return 0
	}
	return i
}
