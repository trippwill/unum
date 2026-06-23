package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/profile"
	"github.com/trippwill/unum/internal/runtime/podman"
)

func TestStatusUsesConfig(t *testing.T) {
	cfg := config.Default()
	cfg.ServerName = "lab"
	cfg.Inference.Enabled = true
	cfg.Inference.Address = ":8770"
	cfg.Inference.BasePath = "/openai/v1/"
	cfg.Inference.DevInsecureHTTP = true
	cfg.Inference.ActiveProfile = "qwen3"

	got, err := New(cfg, "test-version").Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if got.ServerName != "lab" || got.Version != "test-version" || got.RuntimeBackend != "podman" {
		t.Fatalf("unexpected status: %+v", got)
	}
	if got.InferenceEndpoint != "http://localhost:8770/openai/v1" {
		t.Fatalf("InferenceEndpoint = %q", got.InferenceEndpoint)
	}
	if got.ActiveProfile != "qwen3" || got.Operations != "idle" {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestStatusOmitsDisabledInferenceEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.Inference.Enabled = false

	got, err := New(cfg, "test-version").Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.InferenceEndpoint != "" {
		t.Fatalf("InferenceEndpoint = %q", got.InferenceEndpoint)
	}
}

func TestListProfilesUsesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", model)

	cfg := config.Default()
	cfg.Storage.Profiles = dir
	got, err := New(cfg, "test-version").ListProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "qwen" || !got[0].Valid {
		t.Fatalf("profiles = %+v", got)
	}
}

func TestActivateProfileUpdatesStatus(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	svc := New(cfg, "test-version")

	if err := svc.ActivateProfile(context.Background(), "qwen"); err != nil {
		t.Fatal(err)
	}
	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.ActiveProfile != "qwen" {
		t.Fatalf("ActiveProfile = %q", status.ActiveProfile)
	}
}

func TestStartAndStopProfileRecordsOperationsAndInstance(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	runtime := &fakeRuntime{status: podman.ContainerStatus{ID: "container-1", State: "running", Health: "healthy", Started: time.Unix(100, 0)}}
	svc := New(cfg, "test-version", WithRuntimeBackend(runtime))

	start, err := svc.StartProfile(context.Background(), "qwen")
	if err != nil {
		t.Fatal(err)
	}
	if start.State != "succeeded" || start.Phase != "ready" {
		t.Fatalf("start operation = %+v", start)
	}
	if !reflect.DeepEqual(runtime.calls, []string{"ensure:example", "create:qwen", "start:container-1", "inspect:container-1"}) {
		t.Fatalf("runtime calls = %#v", runtime.calls)
	}

	instances, err := svc.ListInstances(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].ProfileID != "qwen" || instances[0].Health != "healthy" {
		t.Fatalf("instances = %+v", instances)
	}
	if instances[0].StartedAt == "" {
		t.Fatalf("StartedAt was not set: %+v", instances[0])
	}

	stop, err := svc.StopProfile(context.Background(), "qwen")
	if err != nil {
		t.Fatal(err)
	}
	if stop.State != "succeeded" || stop.Phase != "stopped" {
		t.Fatalf("stop operation = %+v", stop)
	}
	if !reflect.DeepEqual(runtime.calls, []string{"ensure:example", "create:qwen", "start:container-1", "inspect:container-1", "stop:container-1", "remove:container-1"}) {
		t.Fatalf("runtime calls after stop = %#v", runtime.calls)
	}

	events, err := svc.WatchEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var phases []string
	for event := range events {
		phases = append(phases, event.Phase)
	}
	for _, phase := range []string{"checking image", "creating container", "ready", "stopped"} {
		if !contains(phases, phase) {
			t.Fatalf("events missing %q: %v", phase, phases)
		}
	}
}

func TestTailLogsReadsRuntimeLogs(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	runtime := &fakeRuntime{
		status: podman.ContainerStatus{ID: "container-1", State: "running", Health: "healthy"},
		logs:   []podman.LogLine{{Text: "ready"}, {Text: "served"}},
	}
	svc := New(cfg, "test-version", WithRuntimeBackend(runtime))
	if _, err := svc.StartProfile(context.Background(), "qwen"); err != nil {
		t.Fatal(err)
	}

	got, err := svc.TailLogs(context.Background(), "container-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "ready" || got[1].Text != "served" {
		t.Fatalf("logs = %+v", got)
	}
	if runtime.logTail != 2 {
		t.Fatalf("tail = %d", runtime.logTail)
	}
}

func TestTailLogsRejectsUnknownInstance(t *testing.T) {
	svc := New(config.Default(), "test-version", WithRuntimeBackend(&fakeRuntime{}))
	if _, err := svc.TailLogs(context.Background(), "missing", 10); err == nil {
		t.Fatal("TailLogs accepted unknown instance")
	}
}

func TestStartProfileRejectsInvalidProfile(t *testing.T) {
	dir := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", filepath.Join(t.TempDir(), "missing"))
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	svc := New(cfg, "test-version", WithRuntimeBackend(&fakeRuntime{}))

	op, err := svc.StartProfile(context.Background(), "qwen")
	if err == nil {
		t.Fatal("StartProfile accepted invalid profile")
	}
	if op.State != "failed" || op.Phase != "validating" {
		t.Fatalf("operation = %+v", op)
	}
}

func TestStartProfileRejectsMultiServiceProfileForNow(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeMultiServiceProfile(t, filepath.Join(dir, "stack.yaml"), model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	svc := New(cfg, "test-version", WithRuntimeBackend(&fakeRuntime{}))

	op, err := svc.StartProfile(context.Background(), "stack")
	if err == nil {
		t.Fatal("StartProfile accepted multi-service profile")
	}
	if op.State != "failed" || op.Phase != "validating" {
		t.Fatalf("operation = %+v", op)
	}
	if !strings.Contains(err.Error(), "multi-service start is not implemented") {
		t.Fatalf("error = %v", err)
	}
}

func TestStartProfileRemovesContainerWhenStartFails(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	runtime := &fakeRuntime{startErr: errors.New("boom")}
	svc := New(cfg, "test-version", WithRuntimeBackend(runtime))

	op, err := svc.StartProfile(context.Background(), "qwen")
	if err == nil {
		t.Fatal("StartProfile succeeded")
	}
	if op.State != "failed" || op.Phase != "starting container" {
		t.Fatalf("operation = %+v", op)
	}
	if !reflect.DeepEqual(runtime.calls, []string{"ensure:example", "create:qwen", "start:container-1", "remove:container-1"}) {
		t.Fatalf("runtime calls = %#v", runtime.calls)
	}
}

func TestStartProfileStopsAndRemovesContainerWhenInspectFails(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.yaml"), "qwen", model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	runtime := &fakeRuntime{inspectErr: errors.New("boom")}
	svc := New(cfg, "test-version", WithRuntimeBackend(runtime))

	op, err := svc.StartProfile(context.Background(), "qwen")
	if err == nil {
		t.Fatal("StartProfile succeeded")
	}
	if op.State != "failed" || op.Phase != "waiting for health" {
		t.Fatalf("operation = %+v", op)
	}
	if !reflect.DeepEqual(runtime.calls, []string{"ensure:example", "create:qwen", "start:container-1", "inspect:container-1", "stop:container-1", "remove:container-1"}) {
		t.Fatalf("runtime calls = %#v", runtime.calls)
	}
}

func TestInferenceTokenLifecycle(t *testing.T) {
	cfg := config.Default()
	cfg.Storage.State = t.TempDir()
	svc := New(cfg, "test-version")

	created, err := svc.CreateInferenceToken(context.Background(), "editor")
	if err != nil {
		t.Fatal(err)
	}
	if created.Raw == "" || created.Prefix == "" {
		t.Fatalf("created = %+v", created)
	}
	list, err := svc.ListInferenceTokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "editor" || list[0].Revoked {
		t.Fatalf("list = %+v", list)
	}
	if err := svc.RevokeInferenceToken(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	list, err = svc.ListInferenceTokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !list[0].Revoked {
		t.Fatalf("token was not revoked: %+v", list)
	}
}

type fakeRuntime struct {
	status     podman.ContainerStatus
	calls      []string
	startErr   error
	inspectErr error
	logs       []podman.LogLine
	logTail    int
}

func (f *fakeRuntime) EnsureImage(_ context.Context, image string) error {
	f.calls = append(f.calls, "ensure:"+image)
	return nil
}

func (f *fakeRuntime) Create(_ context.Context, p profile.Profile) (podman.ContainerID, error) {
	f.calls = append(f.calls, "create:"+p.ID)
	return "container-1", nil
}

func (f *fakeRuntime) Start(_ context.Context, id podman.ContainerID) error {
	f.calls = append(f.calls, "start:"+string(id))
	return f.startErr
}

func (f *fakeRuntime) Stop(_ context.Context, id podman.ContainerID) error {
	f.calls = append(f.calls, "stop:"+string(id))
	return nil
}

func (f *fakeRuntime) Remove(_ context.Context, id podman.ContainerID) error {
	f.calls = append(f.calls, "remove:"+string(id))
	return nil
}

func (f *fakeRuntime) Inspect(_ context.Context, id podman.ContainerID) (podman.ContainerStatus, error) {
	f.calls = append(f.calls, "inspect:"+string(id))
	if f.inspectErr != nil {
		return podman.ContainerStatus{}, f.inspectErr
	}
	return f.status, nil
}

func (f *fakeRuntime) Logs(_ context.Context, id podman.ContainerID, opts podman.LogOptions) (<-chan podman.LogLine, error) {
	f.calls = append(f.calls, "logs:"+string(id))
	f.logTail = opts.Tail
	ch := make(chan podman.LogLine, len(f.logs))
	for _, line := range f.logs {
		ch <- line
	}
	close(ch)
	return ch, nil
}

func writeServiceProfile(t *testing.T, path, id, model string) {
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
  models:
    - ` + model + `
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeMultiServiceProfile(t *testing.T, path, model string) {
	t.Helper()
	data := []byte(`services:
  llm:
    image: example-llm
    network_mode: host
    volumes:
      - ` + model + `:/models:ro
    command: ["serve-llm"]
  diffusion:
    image: example-diffusion
    network_mode: host
    command: ["serve-diffusion"]
x-unum:
  id: stack
  name: Stack
  endpoints:
    openai:
      service: llm
      url: http://127.0.0.1:18080/v1
      health: /health
    diffusion:
      service: diffusion
      url: http://127.0.0.1:18100
      health: /health
  models:
    - ` + model + `
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
