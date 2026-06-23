package podman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/trippwill/unum/internal/profile"
)

func TestCreateMapsProfileToPodmanArgs(t *testing.T) {
	var got []string
	backend := Backend{run: func(_ context.Context, args ...string) ([]byte, error) {
		got = append([]string(nil), args...)
		return []byte("abc123\n"), nil
	}}
	p := profile.Profile{
		ID: "qwen3/small cpu",
		Services: map[string]profile.Service{
			"qwen": {
				Image:        "example/llm:latest",
				NetworkMode:  "host",
				Devices:      []string{"/dev/dri/renderD128"},
				Volumes:      []string{"/models:/models:ro"},
				Environment:  map[string]string{"B": "2", "A": "1"},
				MemLimit:     "32g",
				MemswapLimit: "32g",
				ShmSize:      "8g",
				SecurityOpt:  []string{"label=disable"},
				Entrypoint:   "/bin/bash",
				Command:      []string{"serve", "--port", "18080"},
			},
		},
	}

	id, err := backend.Create(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if id != "abc123" {
		t.Fatalf("id = %q", id)
	}
	want := []string{
		"create",
		"--name", "unum-qwen3-small-cpu-e2c8e7f3",
		"--label", "unum.managed=true",
		"--label", "unum.profile=qwen3/small cpu",
		"--network", "host",
		"--memory", "32g",
		"--memory-swap", "32g",
		"--shm-size", "8g",
		"--env", "A=1",
		"--env", "B=2",
		"--volume", "/models:/models:ro",
		"--device", "/dev/dri/renderD128",
		"--security-opt", "label=disable",
		"--entrypoint", "/bin/bash",
		"example/llm:latest",
		"serve", "--port", "18080",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v", got)
	}
}

func TestCreateRemovesExitedNameConflictAndRetries(t *testing.T) {
	var calls [][]string
	backend := Backend{run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch len(calls) {
		case 1:
			return nil, errors.New(`creating container storage: the container name "unum-qwen" is already in use`)
		case 2:
			return []byte(`[{"Id":"old-container","Name":"/unum-qwen","Config":{"Labels":{"unum.managed":"true","unum.profile":"qwen"}},"State":{"Status":"exited"}}]`), nil
		case 4:
			return []byte("new-container\n"), nil
		default:
			return nil, nil
		}
	}}
	p := profile.Profile{
		ID: "qwen",
		Services: map[string]profile.Service{
			"qwen": {Image: "example", ContainerName: "unum-qwen"},
		},
	}

	id, err := backend.Create(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if id != "new-container" {
		t.Fatalf("id = %q", id)
	}
	want := [][]string{
		{"create", "--name", "unum-qwen", "--label", "unum.managed=true", "--label", "unum.profile=qwen", "example"},
		{"inspect", "--type", "container", "unum-qwen"},
		{"rm", "old-container"},
		{"create", "--name", "unum-qwen", "--label", "unum.managed=true", "--label", "unum.profile=qwen", "example"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestCreateDoesNotReplaceRunningNameConflict(t *testing.T) {
	var calls [][]string
	backend := Backend{run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			return nil, errors.New(`creating container storage: the container name "unum-qwen" is already in use`)
		}
		return []byte(`[{"Id":"old-container","Name":"/unum-qwen","Config":{"Labels":{"unum.managed":"true","unum.profile":"qwen"}},"State":{"Status":"running"}}]`), nil
	}}
	p := profile.Profile{
		ID: "qwen",
		Services: map[string]profile.Service{
			"qwen": {Image: "example", ContainerName: "unum-qwen"},
		},
	}

	if _, err := backend.Create(context.Background(), p); err == nil || !strings.Contains(err.Error(), "running container") {
		t.Fatalf("err = %v", err)
	}
	want := [][]string{
		{"create", "--name", "unum-qwen", "--label", "unum.managed=true", "--label", "unum.profile=qwen", "example"},
		{"inspect", "--type", "container", "unum-qwen"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestCreateDoesNotRemoveUnmanagedNameConflict(t *testing.T) {
	var calls [][]string
	backend := Backend{run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			return nil, errors.New(`creating container storage: the container name "unum-qwen" is already in use`)
		}
		return []byte(`[{"Id":"old-container","Name":"/unum-qwen","State":{"Status":"exited"}}]`), nil
	}}
	p := profile.Profile{
		ID: "qwen",
		Services: map[string]profile.Service{
			"qwen": {Image: "example", ContainerName: "unum-qwen"},
		},
	}

	if _, err := backend.Create(context.Background(), p); err == nil || !strings.Contains(err.Error(), "not managed by Unum") {
		t.Fatalf("err = %v", err)
	}
	want := [][]string{
		{"create", "--name", "unum-qwen", "--label", "unum.managed=true", "--label", "unum.profile=qwen", "example"},
		{"inspect", "--type", "container", "unum-qwen"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestEnsureImagePullsWhenMissing(t *testing.T) {
	var calls [][]string
	backend := Backend{run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(calls) == 1 {
			return nil, errFake
		}
		return nil, nil
	}}

	if err := backend.EnsureImage(context.Background(), "example/llm:latest"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"image", "exists", "example/llm:latest"}, {"pull", "example/llm:latest"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestInspectParsesStatus(t *testing.T) {
	backend := Backend{run: func(_ context.Context, args ...string) ([]byte, error) {
		return []byte(`[{"Id":"abc","Name":"/unum-qwen","State":{"Status":"running","StartedAt":"2026-06-22T18:00:00Z","Health":{"Status":"healthy"}}}]`), nil
	}}

	got, err := backend.Inspect(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "abc" || got.Name != "unum-qwen" || got.State != "running" || got.Health != "healthy" || got.Started.IsZero() {
		t.Fatalf("status = %+v", got)
	}
}

func TestLogsReadsStdoutAndStderr(t *testing.T) {
	script := filepath.Join(t.TempDir(), "podman")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'stdout log\\n'\nprintf 'stderr log\\n' >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	lines, err := (Backend{Command: script}).Logs(context.Background(), "container-1", LogOptions{Tail: 2})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for line := range lines {
		if line.Err != nil {
			t.Fatal(line.Err)
		}
		seen[line.Text] = true
	}
	for _, want := range []string{"stdout log", "stderr log"} {
		if !seen[want] {
			t.Fatalf("logs missing %q: %#v", want, seen)
		}
	}
}

func TestContainerNameFallback(t *testing.T) {
	if got := containerName("///"); got != "unum-profile-732c4e97" {
		t.Fatalf("containerName = %q", got)
	}
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake" }
