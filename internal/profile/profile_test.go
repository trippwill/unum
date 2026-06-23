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
	svc := p.Services["qwen3-small-cpu"]
	svc.MemLimit = "33g"
	svc.Devices = []string{"dri/renderD128"}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p)
	if got.Valid {
		t.Fatal("profile unexpectedly valid")
	}
	want := strings.Join(got.Errors, "\n")
	for _, part := range []string{"x-unum.models entry is not accessible", "mem_limit exceeds 32g", "devices host must be absolute"} {
		if !strings.Contains(want, part) {
			t.Fatalf("errors %q do not contain %q", want, part)
		}
	}
}

func TestValidateRejectsSwapBelowMemory(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.MemLimit = "16g"
	svc.MemswapLimit = "8g"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p)
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "memswap_limit cannot be less") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateRejectsBadEndpointService(t *testing.T) {
	p := validProfile(t.TempDir())
	p.Unum.Endpoints["openai"] = Endpoint{Service: "missing", URL: "http://127.0.0.1:18080/v1", Health: "/health"}

	got := Validate(p)
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "references unknown service missing") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateAcceptsVolumeModeOptions(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Volumes = []string{p.Unum.Models[0] + ":/models:ro,Z"}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p)
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestLoadDirReturnsSortedSummaries(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	writeProfile(t, filepath.Join(dir, "b.yaml"), "b", model)
	writeProfile(t, filepath.Join(dir, "a.yml"), "a", model)
	if err := os.WriteFile(filepath.Join(dir, "old.toml"), []byte("id = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

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
		ID:   "qwen3-small-cpu",
		Name: "Qwen3 Small CPU",
		Services: map[string]Service{
			"qwen3-small-cpu": {
				Image:        "docker.io/example/unum-llm-cpu:0.1.0",
				NetworkMode:  "host",
				Volumes:      []string{model + ":/models:ro"},
				MemLimit:     "32g",
				MemswapLimit: "32g",
				Command:      []string{"serve"},
			},
		},
		Unum: UnumMetadata{
			ID:   "qwen3-small-cpu",
			Name: "Qwen3 Small CPU",
			Endpoints: map[string]Endpoint{
				"openai": {Service: "qwen3-small-cpu", URL: "http://127.0.0.1:18080/v1", Health: "/health"},
			},
			Models: []string{model},
		},
	}
}

func writeProfile(t *testing.T, path, id, model string) {
	t.Helper()
	data := []byte(`services:
  ` + id + `:
    image: docker.io/example/unum-llm-cpu:0.1.0
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
