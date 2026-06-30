package podman

import (
	"bufio"
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
	Labels  map[string]string
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
	_, svc, err := p.SingleService()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(svc.Image) == "" {
		return "", fmt.Errorf("profile image is required")
	}
	args := createArgs(p, svc)
	out, err := b.runCommand(ctx, args...)
	if isContainerNameConflict(err) {
		if cleanupErr := b.removeStoppedContainer(ctx, profileContainerName(p, svc), p.ID); cleanupErr != nil {
			return "", cleanupErr
		}
		out, err = b.runCommand(ctx, args...)
	}
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
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
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
		Labels: raw[0].Config.Labels,
	}
	if status.Labels == nil {
		status.Labels = map[string]string{}
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
	output, outputWriter := io.Pipe()
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter
	if err := cmd.Start(); err != nil {
		_ = output.Close()
		_ = outputWriter.Close()
		return nil, fmt.Errorf("start podman logs: %w", err)
	}
	lines := make(chan LogLine, 128)
	go scanLogs(ctx, output, outputWriter, lines, cmd)
	return lines, nil
}

func createArgs(p profile.Profile, svc profile.Service) []string {
	args := []string{
		"create",
		"--name", profileContainerName(p, svc),
		"--label", "unum.managed=true",
		"--label", "unum.profile=" + p.ID,
	}
	if svc.NetworkMode != "" {
		args = append(args, "--network", svc.NetworkMode)
	}
	if svc.MemLimit != "" {
		args = append(args, "--memory", svc.MemLimit)
	}
	if svc.MemswapLimit != "" {
		args = append(args, "--memory-swap", svc.MemswapLimit)
	}
	if strings.TrimSpace(svc.Cpus) != "" {
		args = append(args, "--cpus", strings.TrimSpace(svc.Cpus))
	}
	if svc.ShmSize != "" {
		args = append(args, "--shm-size", svc.ShmSize)
	}
	if svc.OOMScoreAdj != nil {
		args = append(args, "--oom-score-adj", fmt.Sprint(*svc.OOMScoreAdj))
	}
	envNames := make([]string, 0, len(svc.Environment))
	for name := range svc.Environment {
		envNames = append(envNames, name)
	}
	sort.Strings(envNames)
	for _, name := range envNames {
		args = append(args, "--env", name+"="+svc.Environment[name])
	}
	for _, volume := range svc.Volumes {
		if volume.Short != "" {
			args = append(args, "--volume", volume.Short)
			continue
		}
		args = append(args, "--mount", bindMountArg(volume))
	}
	for _, device := range svc.Devices {
		args = append(args, "--device", device)
	}
	for _, opt := range svc.SecurityOpt {
		args = append(args, "--security-opt", opt)
	}
	if svc.Entrypoint != "" {
		args = append(args, "--entrypoint", svc.Entrypoint)
	}
	args = append(args, svc.Image)
	args = append(args, svc.Command...)
	return args
}

func bindMountArg(volume profile.Volume) string {
	parts := []string{"type=bind", "source=" + volume.Source, "target=" + volume.Target}
	if volume.ReadOnly {
		parts = append(parts, "readonly")
	}
	return strings.Join(parts, ",")
}

func (b Backend) removeStoppedContainer(ctx context.Context, name, profileID string) error {
	status, err := b.Inspect(ctx, ContainerID(name))
	if err != nil {
		if strings.Contains(err.Error(), "no such container") {
			return nil
		}
		return fmt.Errorf("inspect existing container %q: %w", name, err)
	}
	if status.Labels["unum.managed"] != "true" || status.Labels["unum.profile"] != profileID {
		return fmt.Errorf("container name %q is already used by container %s not managed by Unum profile %q", name, status.ID, profileID)
	}
	switch status.State {
	case "configured", "created", "exited", "stopped":
		if err := b.Remove(ctx, status.ID); err != nil {
			return fmt.Errorf("remove stopped container %q (%s): %w", name, status.ID, err)
		}
		return nil
	default:
		return fmt.Errorf("container name %q is already used by %s container %s", name, status.State, status.ID)
	}
}

func isContainerNameConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "container name") && strings.Contains(msg, "already in use")
}

func profileContainerName(p profile.Profile, svc profile.Service) string {
	if svc.ContainerName != "" {
		return svc.ContainerName
	}
	return containerName(p.ID)
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

func scanLogs(ctx context.Context, output *io.PipeReader, outputWriter *io.PipeWriter, lines chan<- LogLine, cmd *exec.Cmd) {
	defer close(lines)
	waitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		_ = outputWriter.Close()
		waitErr <- err
	}()
	defer func() {
		_ = output.Close()
	}()
	lastLine := ""
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		lastLine = scanner.Text()
		select {
		case lines <- LogLine{Text: lastLine}:
		case <-ctx.Done():
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		lines <- LogLine{Err: fmt.Errorf("read podman logs: %w", err)}
	}
	if err := <-waitErr; err != nil && ctx.Err() == nil {
		detail := strings.TrimSpace(lastLine)
		if detail != "" {
			err = fmt.Errorf("%w: %s", err, detail)
		}
		lines <- LogLine{Err: fmt.Errorf("podman logs exited: %w", err)}
	}
}
