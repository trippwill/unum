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

	got := Validate(p, testValidationOptions())
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestValidateRejectsOversizedMemoryForConfiguredLimit(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.MemLimit = "33g"
	svc.Devices = []string{"dri/renderD128"}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if got.Valid {
		t.Fatal("profile unexpectedly valid")
	}
	want := strings.Join(got.Errors, "\n")
	for _, part := range []string{"mem_limit exceeds configured memory_max 32g", "devices host must be absolute"} {
		if !strings.Contains(want, part) {
			t.Fatalf("errors %q do not contain %q", want, part)
		}
	}
}

func TestValidateUsesConfigurableMemoryLimit(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.MemLimit = "64g"
	svc.MemswapLimit = "128g"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, ValidationOptions{MemoryMax: "128g", MemswapMax: "128g"})
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestValidateRejectsSwapOverMemswapMax(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.MemLimit = "16g"
	svc.MemswapLimit = "64g"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, ValidationOptions{MemoryMax: "32g", MemswapMax: "32g"})
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "memswap_limit exceeds configured memswap_max 32g") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateRejectsCPUsOverCeiling(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Cpus = "32"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, ValidationOptions{MemoryMax: "32g", MemswapMax: "32g", CPUsMax: "16"})
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "cpus exceeds configured cpus_max 16") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateAcceptsCPUsWithinCeiling(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Cpus = "8"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, ValidationOptions{MemoryMax: "32g", MemswapMax: "32g", CPUsMax: "16"})
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestValidateSkipsCPUsCheckWhenCeilingZero(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Cpus = "999"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, ValidationOptions{MemoryMax: "32g", MemswapMax: "32g", CPUsMax: "0"})
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestValidateRejectsZeroOrNegativeCPUs(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Cpus = "0"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "cpus must be greater than 0") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateAcceptsRegisteredDevice(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Devices = []string{"/dev/dri/renderD129:/dev/dri/renderD129"}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, ValidationOptions{
		MemoryMax:  "32g",
		MemswapMax: "32g",
		Devices:    []string{"/dev/dri/renderD129"},
	})
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestValidateRejectsUnregisteredDevice(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Devices = []string{"/dev/dri/renderD129"}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if got.Valid {
		t.Fatal("profile unexpectedly valid with empty device registry")
	}
	if !strings.Contains(strings.Join(got.Errors, "\n"), "/dev/dri/renderD129 is not in [inventory].devices") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateRejectsUnregisteredDeviceBindMount(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Volumes = Volumes{
		{Type: "bind", Source: "/dev/dri/card0", Target: "/dev/dri/card0"},
	}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if got.Valid {
		t.Fatal("profile unexpectedly valid: /dev/* bind mount bypassed registry")
	}
	if !strings.Contains(strings.Join(got.Errors, "\n"), "device path /dev/dri/card0 is not in [inventory].devices") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateRejectsSwapBelowMemory(t *testing.T) {
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.MemLimit = "16g"
	svc.MemswapLimit = "8g"
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "memswap_limit cannot be less") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateRejectsBadEndpointService(t *testing.T) {
	p := validProfile(t.TempDir())
	p.Unum.Endpoints["openai"] = Endpoint{Service: "missing", URL: "http://127.0.0.1:18080/v1", Health: "/health"}

	got := Validate(p, testValidationOptions())
	if got.Valid || !strings.Contains(strings.Join(got.Errors, "\n"), "references unknown service missing") {
		t.Fatalf("errors = %v", got.Errors)
	}
}

func TestValidateAcceptsVolumeModeOptions(t *testing.T) {
	model := t.TempDir()
	p := validProfile(model)
	svc := p.Services["qwen3-small-cpu"]
	svc.Volumes = Volumes{{Short: model + ":/models:ro,Z"}}
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestLoadFileAcceptsLongFormBindVolumesAndOOMScore(t *testing.T) {
	dir := t.TempDir()
	model := t.TempDir()
	path := filepath.Join(dir, "qwen.yaml")
	data := []byte(`services:
  qwen:
    image: example
    volumes:
      - type: bind
        source: ` + model + `
        target: /models
        read_only: true
    oom_score_adj: 900
    command: ["serve"]
x-unum:
  id: qwen
  name: qwen
  endpoints:
    openai:
      service: qwen
      url: http://127.0.0.1:18080/v1
      health: /health
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := p.Services["qwen"]
	if len(svc.Volumes) != 1 || svc.Volumes[0].Source != model || !svc.Volumes[0].ReadOnly {
		t.Fatalf("volumes = %+v", svc.Volumes)
	}
	if svc.OOMScoreAdj == nil || *svc.OOMScoreAdj != 900 {
		t.Fatalf("oom_score_adj = %v", svc.OOMScoreAdj)
	}
	if got := Validate(p, testValidationOptions()); !got.Valid {
		t.Fatalf("Validate errors = %v", got.Errors)
	}
}

func TestQwen3CoderB60ExampleLoads(t *testing.T) {
	p, err := LoadFile("../../examples/profiles/qwen3-coder-b60.yaml")
	if err != nil {
		t.Fatal(err)
	}
	svc := p.Services["qwen3-coder-b60"]
	if svc.OOMScoreAdj == nil || *svc.OOMScoreAdj != 900 {
		t.Fatalf("oom_score_adj = %v", svc.OOMScoreAdj)
	}
	if len(svc.Volumes) == 0 || svc.Volumes[0].Type != "bind" {
		t.Fatalf("volumes = %+v", svc.Volumes)
	}
	got := Validate(p, b60ValidationOptions())
	if !got.Valid {
		t.Fatalf("example schema errors: %v", got.Errors)
	}
}

func TestValidateRejectsBadLongFormVolumeAndOOMScore(t *testing.T) {
	oomScoreAdj := 1001
	p := validProfile(t.TempDir())
	svc := p.Services["qwen3-small-cpu"]
	svc.Volumes = Volumes{{Type: "volume", Source: "relative", Target: "models"}}
	svc.OOMScoreAdj = &oomScoreAdj
	p.Services["qwen3-small-cpu"] = svc

	got := Validate(p, testValidationOptions())
	if got.Valid {
		t.Fatal("profile unexpectedly valid")
	}
	want := strings.Join(got.Errors, "\n")
	for _, part := range []string{"long form type must be bind", "long form source must be absolute", "long form target must be absolute", "oom_score_adj must be between"} {
		if !strings.Contains(want, part) {
			t.Fatalf("errors %q do not contain %q", want, part)
		}
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

	got, err := LoadDir(dir, testValidationOptions())
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
				Volumes:      Volumes{{Short: model + ":/models:ro"}},
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
		},
	}
}

func testValidationOptions() ValidationOptions {
	return ValidationOptions{
		MemoryMax:  "32g",
		MemswapMax: "32g",
	}
}

func b60ValidationOptions() ValidationOptions {
	return ValidationOptions{
		MemoryMax:  "32g",
		MemswapMax: "64g",
		Devices: []string{
			"/dev/dri/renderD129",
			"/dev/dri/card0",
			"/dev/dri/by-path/pci-0000:12:00.0-card",
			"/dev/dri/by-path/pci-0000:12:00.0-render",
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
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
