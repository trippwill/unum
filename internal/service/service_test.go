package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

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
	if err := os.WriteFile(filepath.Join(dir, "qwen.toml"), []byte(`id = "qwen"
name = "Qwen"

[runtime]
backend = "podman"

[image]
ref = "example"

[model]
path = "`+model+`"

[server]
kind = "openai-compatible"
host = "127.0.0.1"
port = 18080
health_path = "/health"

[container]
network = "host"
args = ["serve"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

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

func TestStartAndStopProfileRecordsOperationsAndInstance(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.toml"), "qwen", model)
	cfg := config.Default()
	cfg.Storage.Profiles = dir
	runtime := &fakeRuntime{status: podman.ContainerStatus{ID: "container-1", State: "running", Health: "healthy"}}
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

func TestStartProfileRejectsInvalidProfile(t *testing.T) {
	dir := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.toml"), "qwen", filepath.Join(t.TempDir(), "missing"))
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

func TestStartProfileRemovesContainerWhenStartFails(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeServiceProfile(t, filepath.Join(dir, "qwen.toml"), "qwen", model)
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
	writeServiceProfile(t, filepath.Join(dir, "qwen.toml"), "qwen", model)
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

func writeServiceProfile(t *testing.T, path, id, model string) {
	t.Helper()
	data := []byte(`id = "` + id + `"
name = "` + id + `"

[runtime]
backend = "podman"

[image]
ref = "example"

[model]
path = "` + model + `"

[server]
kind = "openai-compatible"
host = "127.0.0.1"
port = 18080
health_path = "/health"

[container]
network = "host"
args = ["serve"]
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
