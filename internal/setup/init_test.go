package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/trippwill/unum/internal/config"
)

func TestInitCreatesConfigStateHostKeyAndProfile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "etc", "unumd.toml")
	state := filepath.Join(dir, "state")

	if err := Init(InitOptions{ConfigPath: cfgPath, StateDir: state, ServerName: "lab"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "lab" {
		t.Fatalf("ServerName = %q", cfg.ServerName)
	}

	profilePath := filepath.Join(state, "profiles", "qwen3-small-cpu.yaml")
	for _, path := range []string{
		profilePath,
		filepath.Join(state, "ssh", "host_ed25519"),
		filepath.Join(state, "tokens"),
		filepath.Join(state, "logs"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	modelsDir := filepath.Join(state, "models")
	if !strings.Contains(string(profile), modelsDir+":/models:ro") {
		t.Fatalf("profile does not use configured models directory %q", modelsDir)
	}

	key, err := os.ReadFile(filepath.Join(state, "ssh", "host_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParseRawPrivateKey(key); err != nil {
		t.Fatalf("host key is not a private key: %v", err)
	}
}

func TestInitDoesNotOverwriteExistingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
	state := filepath.Join(dir, "state")
	if err := os.WriteFile(cfgPath, []byte("server_name = \"keep\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Init(InitOptions{ConfigPath: cfgPath, StateDir: state, ServerName: "replace"}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "keep" {
		t.Fatalf("ServerName = %q", cfg.ServerName)
	}
}
