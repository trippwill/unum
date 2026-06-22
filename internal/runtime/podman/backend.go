package podman

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/trippwill/unum/internal/profile"
)

type Backend struct {
	Command string
	run     func(context.Context, ...string) ([]byte, error)
}

type RuntimeInfo struct {
	Name    string
	Version string
}

type ContainerID string

type ContainerStatus struct {
	ID      ContainerID
	Name    string
	State   string
	Health  string
	Started time.Time
}

type LogOptions struct {
	Tail   int
	Follow bool
}

type LogLine struct {
	Text string
	Err  error
}

func New() Backend {
	return Backend{Command: "podman"}
}

func (b Backend) Probe(ctx context.Context) (RuntimeInfo, error) {
	out, err := b.runCommand(ctx, "version", "--format", "json")
	if err != nil {
		return RuntimeInfo{}, err
	}
	var version struct {
		Client struct {
			Version string `json:"Version"`
		} `json:"Client"`
	}
	if err := json.Unmarshal(out, &version); err != nil {
		return RuntimeInfo{}, fmt.Errorf("parse podman version: %w", err)
	}
	return RuntimeInfo{Name: "podman", Version: version.Client.Version}, nil
}

func (b Backend) EnsureImage(ctx context.Context, image string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image is required")
	}
	if _, err := b.runCommand(ctx, "image", "exists", image); err == nil {
		return nil
	}
	_, err := b.runCommand(ctx, "pull", image)
	return err
}

func (b Backend) Create(ctx context.Context, p profile.Profile) (ContainerID, error) {
	if strings.TrimSpace(p.Image.Ref) == "" {
		return "", fmt.Errorf("profile image is required")
	}
	args := createArgs(p)
	out, err := b.runCommand(ctx, args...)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("podman create returned empty container id")
	}
	return ContainerID(id), nil
}

func (b Backend) Start(ctx context.Context, id ContainerID) error {
	_, err := b.runCommand(ctx, "start", string(id))
	return err
}

func (b Backend) Stop(ctx context.Context, id ContainerID) error {
	_, err := b.runCommand(ctx, "stop", string(id))
	return err
}

func (b Backend) Remove(ctx context.Context, id ContainerID) error {
	_, err := b.runCommand(ctx, "rm", string(id))
	return err
}

func (b Backend) Inspect(ctx context.Context, id ContainerID) (ContainerStatus, error) {
	out, err := b.runCommand(ctx, "inspect", "--type", "container", string(id))
	if err != nil {
		return ContainerStatus{}, err
	}
	var raw []struct {
		ID    string `json:"Id"`
		Name  string `json:"Name"`
		State struct {
			Status    string `json:"Status"`
			StartedAt string `json:"StartedAt"`
			Health    *struct {
				Status string `json:"Status"`
			} `json:"Health"`
		} `json:"State"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return ContainerStatus{}, fmt.Errorf("parse podman inspect: %w", err)
	}
	if len(raw) == 0 {
		return ContainerStatus{}, fmt.Errorf("podman inspect returned no containers")
	}
	status := ContainerStatus{
		ID:     ContainerID(raw[0].ID),
		Name:   strings.TrimPrefix(raw[0].Name, "/"),
		State:  raw[0].State.Status,
		Health: "unknown",
	}
	if raw[0].State.Health != nil && raw[0].State.Health.Status != "" {
		status.Health = raw[0].State.Health.Status
	}
	if raw[0].State.StartedAt != "" {
		started, err := time.Parse(time.RFC3339Nano, raw[0].State.StartedAt)
		if err == nil {
			status.Started = started
		}
	}
	return status, nil
}

func (b Backend) Logs(ctx context.Context, id ContainerID, opts LogOptions) (<-chan LogLine, error) {
	args := []string{"logs"}
	if opts.Tail > 0 {
		args = append(args, "--tail", fmt.Sprint(opts.Tail))
	}
	if opts.Follow {
		args = append(args, "--follow")
	}
	args = append(args, string(id))

	cmd := exec.CommandContext(ctx, b.command(), args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open podman logs stdout: %w", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start podman logs: %w", err)
	}
	lines := make(chan LogLine, 128)
	go scanLogs(ctx, stdout, lines, cmd, stderr)
	return lines, nil
}

func createArgs(p profile.Profile) []string {
	args := []string{
		"create",
		"--name", containerName(p.ID),
		"--label", "unum.managed=true",
		"--label", "unum.profile=" + p.ID,
	}
	if p.Container.Network != "" {
		args = append(args, "--network", p.Container.Network)
	}
	if p.Resources.Memory != "" {
		args = append(args, "--memory", p.Resources.Memory)
	}
	if p.Resources.MemorySwap != "" {
		args = append(args, "--memory-swap", p.Resources.MemorySwap)
	}
	mountNames := make([]string, 0, len(p.Mounts))
	for name := range p.Mounts {
		mountNames = append(mountNames, name)
	}
	sort.Strings(mountNames)
	for _, name := range mountNames {
		mount := p.Mounts[name]
		mode := "rw"
		if mount.ReadOnly {
			mode = "ro"
		}
		args = append(args, "--volume", mount.Host+":"+mount.Container+":"+mode)
	}
	for _, device := range p.Container.Devices {
		args = append(args, "--device", device)
	}
	args = append(args, p.Image.Ref)
	args = append(args, p.Container.Args...)
	return args
}

func containerName(profileID string) string {
	safe := regexp.MustCompile(`[^A-Za-z0-9_.-]+`).ReplaceAllString(profileID, "-")
	safe = strings.Trim(safe, "-")
	if safe == "" {
		safe = "profile"
	}
	sum := sha256.Sum256([]byte(profileID))
	return "unum-" + safe + "-" + hex.EncodeToString(sum[:4])
}

func (b Backend) runCommand(ctx context.Context, args ...string) ([]byte, error) {
	if b.run != nil {
		return b.run(ctx, args...)
	}
	cmd := exec.CommandContext(ctx, b.command(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("podman %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (b Backend) command() string {
	if b.Command == "" {
		return "podman"
	}
	return b.Command
}

func scanLogs(ctx context.Context, stdout io.Reader, lines chan<- LogLine, cmd *exec.Cmd, stderr *bytes.Buffer) {
	defer close(lines)
	waited := false
	wait := func() error {
		waited = true
		return cmd.Wait()
	}
	defer func() {
		if !waited {
			_ = cmd.Wait()
		}
	}()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case lines <- LogLine{Text: scanner.Text()}:
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil {
		lines <- LogLine{Err: fmt.Errorf("read podman logs: %w", err)}
	}
	if err := wait(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			err = fmt.Errorf("%w: %s", err, detail)
		}
		lines <- LogLine{Err: fmt.Errorf("podman logs exited: %w", err)}
	}
}
