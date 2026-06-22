package podman

import (
	"context"
	"reflect"
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
		ID:      "qwen3/small cpu",
		Image:   profile.Image{Ref: "example/llm:latest"},
		Runtime: profile.Runtime{Backend: "podman"},
		Resources: profile.Resources{
			Memory:     "32g",
			MemorySwap: "32g",
		},
		Mounts: map[string]profile.Mount{
			"models": {Host: "/models", Container: "/models", ReadOnly: true},
		},
		Container: profile.Container{
			Network: "host",
			Devices: []string{"/dev/dri/renderD128"},
			Args:    []string{"serve", "--port", "18080"},
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
		"--volume", "/models:/models:ro",
		"--device", "/dev/dri/renderD128",
		"example/llm:latest",
		"serve", "--port", "18080",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v", got)
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

func TestContainerNameFallback(t *testing.T) {
	if got := containerName("///"); got != "unum-profile-732c4e97" {
		t.Fatalf("containerName = %q", got)
	}
}

var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake" }
