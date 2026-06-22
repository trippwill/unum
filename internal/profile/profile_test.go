package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAcceptsMinimalCPUProfile(t *testing.T) {
	model := t.TempDir()
	p := validProfile(model)

	got := Validate(p)
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestValidateRejectsMissingModelAndOversizedMemory(t *testing.T) {
	p := validProfile(filepath.Join(t.TempDir(), "missing"))
	p.Resources.Memory = "33g"
	p.Container.Devices = []string{"dri/renderD128"}

	got := Validate(p)
	if got.Valid {
		t.Fatal("profile unexpectedly valid")
	}
	want := strings.Join(got.Errors, "\n")
	for _, part := range []string{"model.path is not accessible", "resources.memory exceeds 32g", "container.devices entries must be absolute"} {
		if !strings.Contains(want, part) {
			t.Fatalf("errors %q do not contain %q", want, part)
		}
	}
}

func TestValidateRejectsSwapBelowMemory(t *testing.T) {
	p := validProfile(t.TempDir())
	p.Resources.Memory = "16g"
	p.Resources.MemorySwap = "8g"

	got := Validate(p)
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "memory_swap cannot be less") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestLoadDirReturnsSortedSummaries(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeProfile(t, filepath.Join(dir, "b.toml"), "b", model)
	writeProfile(t, filepath.Join(dir, "a.toml"), "a", model)

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("unexpected summaries: %+v", got)
	}
}

func validProfile(model string) Profile {
	return Profile{
		ID:      "qwen3-small-cpu",
		Name:    "Qwen3 Small CPU",
		Runtime: Runtime{Backend: "podman"},
		Image:   Image{Ref: "docker.io/example/unum-llm-cpu:0.1.0"},
		Model:   Model{Path: model},
		Server:  Server{Kind: "openai-compatible", Host: "127.0.0.1", Port: 18080, HealthPath: "/health"},
		Resources: Resources{
			Memory:     "32g",
			MemorySwap: "32g",
			Threads:    8,
		},
		Mounts: map[string]Mount{
			"models": {Host: model, Container: "/models", ReadOnly: true},
		},
		Container: Container{Network: "host", Args: []string{"serve"}},
	}
}

func writeProfile(t *testing.T, path, id, model string) {
	t.Helper()
	data := []byte(`id = "` + id + `"
name = "` + id + `"

[runtime]
backend = "podman"

[image]
ref = "docker.io/example/unum-llm-cpu:0.1.0"

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
