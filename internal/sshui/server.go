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
			bubbletea.Middleware(func(session charmssh.Session) (tea.Model, []tea.ProgramOption) {
				return newDashboardModelWithContext(session.Context(), svc), []tea.ProgramOption{tea.WithAltScreen()}
			}),
			activeterm.Middleware(),
			logging.Middleware(),
		),
	)
}

type dashboardModel struct {
	svc            *service.Service
	page           page
	status         service.Status
	profiles       []service.ProfileSummary
	profileIndex   int
	instances      []service.InstanceSummary
	instanceIndex  int
	operations     []service.OperationSummary
	operationIndex int
	tokens         []service.InferenceTokenSummary
	tokenIndex     int
	logs           []service.LogLine
	logInstance    service.InstanceSummary
	hasLogInstance bool
	logStream      <-chan service.LogLine
	cancelLogs     context.CancelFunc
	sessionCtx     context.Context
	logOffset      int
	logFollow      bool
	message        string
	err            error
	width          int
	height         int
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

const maxLogLines = 2000

type logLineMsg struct {
	line   service.LogLine
	stream <-chan service.LogLine
}

type logStreamClosedMsg struct {
	instanceID string
	stream     <-chan service.LogLine
}

func newDashboardModel(svc *service.Service) dashboardModel {
	return newDashboardModelWithContext(context.Background(), svc)
}

func newDashboardModelWithContext(ctx context.Context, svc *service.Service) dashboardModel {
	if ctx == nil {
		ctx = context.Background()
	}
	return dashboardModel{svc: svc, sessionCtx: ctx}.refresh()
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case logLineMsg:
		return m.handleLogLine(msg.line, msg.stream)
	case logStreamClosedMsg:
		if !m.isCurrentLogStream(msg.instanceID, msg.stream) {
			return m, nil
		}
		m.logStream = nil
		m.cancelLogs = nil
		m.message = "log stream ended for " + instanceLabel(m.logInstance)
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.logFollow {
			m.logOffset = maxLogOffset(len(m.logs), m.logBodyHeight())
		} else {
			m.clampLogOffset()
		}
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
		case "s":
			m = m.startProfile()
		case "x":
			m = m.stopOrRevoke()
		case "l":
			var cmd tea.Cmd
			m, cmd = m.loadLogs()
			return m, cmd
		case "f":
			m = m.toggleLogFollow()
		case "c":
			m = m.createToken()
		case "q", "ctrl+c":
			m = m.stopLogStream()
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
	m.operationIndex = clamp(m.operationIndex, len(m.operations))
	m.tokenIndex = clamp(m.tokenIndex, len(m.tokens))
	return m
}

func (m *dashboardModel) move(delta int) {
	switch m.page {
	case pageProfiles:
		m.profileIndex = moveIndex(m.profileIndex, delta, len(m.profiles))
	case pageInstances:
		m.instanceIndex = moveIndex(m.instanceIndex, delta, len(m.instances))
	case pageLogs:
		m.scrollLogs(delta)
	case pageOperations:
		m.operationIndex = moveIndex(m.operationIndex, delta, len(m.operations))
	case pageTokens:
		m.tokenIndex = moveIndex(m.tokenIndex, delta, len(m.tokens))
	}
}

func (m *dashboardModel) scrollLogs(delta int) {
	if delta < 0 && m.logFollow {
		m.logFollow = false
		m.message = "paused log follow for " + instanceLabel(m.logInstance)
	}
	m.logOffset += delta
	m.clampLogOffset()
}

func (m dashboardModel) startProfile() dashboardModel {
	if m.page != pageProfiles || len(m.profiles) == 0 {
		return m
	}
	id := m.profiles[m.profileIndex].ID
	op, err := m.svc.StartProfile(context.Background(), id)
	if err != nil {
		return m.showOperationError(op)
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
			return m.showOperationError(op)
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

func (m dashboardModel) showOperationError(op service.OperationSummary) dashboardModel {
	m = m.refresh()
	m.page = pageOperations
	m.selectOperation(op.ID)
	m.message = op.State + " " + op.Phase + "; see operation detail"
	return m
}

func (m *dashboardModel) selectOperation(id string) {
	m.operationIndex = clamp(m.operationIndex, len(m.operations))
	for i, op := range m.operations {
		if op.ID == id {
			m.operationIndex = i
			return
		}
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

func (m dashboardModel) loadLogs() (dashboardModel, tea.Cmd) {
	instance, ok := m.logTarget()
	if !ok {
		m.message = "no instances; start a profile first"
		return m, nil
	}
	m = m.stopLogStream()
	ctx, cancel := context.WithCancel(m.logContext())
	stream, err := m.svc.StreamLogs(ctx, instance.ID, service.LogOptions{Tail: 100, Follow: true})
	if err != nil {
		cancel()
		m.err = fmt.Errorf("logs for %s: %w", instanceLabel(instance), err)
		return m, nil
	}
	m.page = pageLogs
	m.logs = nil
	m.logInstance = instance
	m.hasLogInstance = true
	m.logStream = stream
	m.cancelLogs = cancel
	m.logOffset = 0
	m.logFollow = true
	m.err = nil
	m.message = "streaming logs for " + instanceLabel(instance)
	return m, waitForLogLine(instance.ID, stream)
}

func (m dashboardModel) logContext() context.Context {
	if m.sessionCtx != nil {
		return m.sessionCtx
	}
	return context.Background()
}

func (m dashboardModel) logTarget() (service.InstanceSummary, bool) {
	if m.page == pageLogs && m.hasLogInstance {
		return m.logInstance, true
	}
	if len(m.instances) == 0 {
		return service.InstanceSummary{}, false
	}
	return m.instances[m.instanceIndex], true
}

func (m dashboardModel) stopLogStream() dashboardModel {
	if m.cancelLogs != nil {
		m.cancelLogs()
	}
	m.cancelLogs = nil
	m.logStream = nil
	return m
}

func waitForLogLine(instanceID string, stream <-chan service.LogLine) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-stream
		if !ok {
			return logStreamClosedMsg{instanceID: instanceID, stream: stream}
		}
		return logLineMsg{line: line, stream: stream}
	}
}

func (m dashboardModel) handleLogLine(line service.LogLine, stream <-chan service.LogLine) (dashboardModel, tea.Cmd) {
	if !m.isCurrentLogStream(line.InstanceID, stream) {
		return m, nil
	}
	if line.Err != nil {
		m.err = fmt.Errorf("logs for %s: %w", instanceLabel(m.logInstance), line.Err)
		m = m.stopLogStream()
		return m, nil
	}
	m.logs = append(m.logs, line)
	if len(m.logs) > maxLogLines {
		drop := len(m.logs) - maxLogLines
		m.logs = m.logs[drop:]
		m.logOffset -= drop
		if m.logOffset < 0 {
			m.logOffset = 0
		}
	}
	if m.logFollow {
		m.logOffset = maxLogOffset(len(m.logs), m.logBodyHeight())
	} else {
		m.clampLogOffset()
	}
	if m.logStream == nil {
		return m, nil
	}
	return m, waitForLogLine(m.logInstance.ID, m.logStream)
}

func (m dashboardModel) isCurrentLogStream(instanceID string, stream <-chan service.LogLine) bool {
	return m.hasLogInstance && instanceID == m.logInstance.ID && stream == m.logStream
}

func (m dashboardModel) toggleLogFollow() dashboardModel {
	if m.page != pageLogs || !m.hasLogInstance {
		return m
	}
	m.logFollow = !m.logFollow
	if m.logFollow {
		m.logOffset = maxLogOffset(len(m.logs), m.logBodyHeight())
		m.message = "following logs for " + instanceLabel(m.logInstance)
	} else {
		m.message = "paused log follow for " + instanceLabel(m.logInstance)
	}
	return m
}

func (m dashboardModel) viewDashboard() string {
	rows := []string{
		"Server:     " + m.status.ServerName,
		"Version:    " + m.status.Version,
		"Runtime:    " + m.status.RuntimeBackend,
		"SSH:        " + m.status.SSHAddress,
		"Inference:  " + m.status.InferenceEndpoint,
		"Running:    " + emptyDash(m.status.RunningProfile),
		"Operations: " + m.status.Operations,
	}
	return strings.Join(rows, "\n")
}

func (m dashboardModel) viewProfiles() string {
	if len(m.profiles) == 0 {
		return "Profiles\n\n(no profiles)\n\ns start  x stop"
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
	return strings.Join(rows, "\n") + "\n\nj/k select  s start  x stop"
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
	if !m.hasLogInstance {
		return "Logs\n\n(no instance selected)\n\nOpen Instances, select an instance, press l."
	}
	follow := "paused"
	if m.logFollow {
		follow = "following"
	}
	rows := []string{
		"Logs",
		"",
		m.clipLine(fmt.Sprintf("%s  profile=%s  id=%s  %s", instanceLabel(m.logInstance), emptyDash(m.logInstance.ProfileID), shortID(m.logInstance.ID), follow)),
		"",
	}
	if len(m.logs) == 0 {
		rows = append(rows, "waiting for logs from "+instanceLabel(m.logInstance))
	} else {
		start, end := visibleRange(len(m.logs), m.logBodyHeight(), m.logOffset)
		rows = append(rows, fmt.Sprintf("lines %d-%d of %d", start+1, end, len(m.logs)))
		for _, line := range m.logs[start:end] {
			rows = append(rows, m.clipLine(line.Text))
		}
	}
	return strings.Join(rows, "\n") + "\n\nj/k scroll  f follow/pause  l reload"
}

func (m dashboardModel) viewOperations() string {
	if len(m.operations) == 0 {
		return "Operations\n\n(no operations)"
	}
	index := clamp(m.operationIndex, len(m.operations))
	op := m.operations[index]
	rows := []string{"Operation Detail", "----------------"}
	for _, line := range []string{
		"ID: " + op.ID,
		"Target: " + op.Target,
		"State: " + op.State,
		"Phase: " + op.Phase,
		"Message: " + emptyDash(op.Message),
	} {
		rows = append(rows, m.wrapLine(line)...)
	}
	rows = append(rows, "", "Operations", "")
	for i, op := range m.operations {
		marker := " "
		if i == index {
			marker = ">"
		}
		rows = append(rows, m.clipLine(fmt.Sprintf("%s %s  %s  %s  %s", marker, op.ID, op.Target, op.State, op.Phase)))
	}
	return strings.Join(rows, "\n") + "\n\nj/k select"
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

func (m dashboardModel) logBodyHeight() int {
	if m.height <= 0 {
		return 10
	}
	height := m.height - 16
	if height < 3 {
		return 3
	}
	return height
}

func (m *dashboardModel) clampLogOffset() {
	if m.logOffset < 0 {
		m.logOffset = 0
	}
	maxOffset := maxLogOffset(len(m.logs), m.logBodyHeight())
	if m.logOffset > maxOffset {
		m.logOffset = maxOffset
	}
}

func maxLogOffset(total, height int) int {
	if height <= 0 || total <= height {
		return 0
	}
	return total - height
}

func visibleRange(total, height, offset int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	if height <= 0 || height > total {
		height = total
	}
	maxOffset := maxLogOffset(total, height)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset, offset + height
}

func (m dashboardModel) clipLine(line string) string {
	if m.width <= 0 {
		return line
	}
	if m.width <= 3 {
		runes := []rune(line)
		if len(runes) <= m.width {
			return line
		}
		return string(runes[:m.width])
	}
	runes := []rune(line)
	if len(runes) <= m.width {
		return line
	}
	return string(runes[:m.width-3]) + "..."
}

func (m dashboardModel) wrapLine(line string) []string {
	if m.width <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) <= m.width {
		return []string{line}
	}
	var lines []string
	for len(runes) > m.width {
		lines = append(lines, string(runes[:m.width]))
		runes = runes[m.width:]
	}
	if len(runes) > 0 {
		lines = append(lines, string(runes))
	}
	return lines
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
