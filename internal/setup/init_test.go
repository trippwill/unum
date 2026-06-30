package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/trippwill/unum/internal/config"
)

func TestEffectiveConfigAppliesDefaults(t *testing.T) {
	cfg, path := effectiveConfig(InitOptions{})
	if path != config.DefaultPath {
		t.Fatalf("config path = %q", path)
	}
	def := config.Default()
	if cfg.Storage != def.Storage {
		t.Fatalf("storage drifted from defaults: %+v vs %+v", cfg.Storage, def.Storage)
	}
	if cfg.SSHTUI.HostKey != filepath.Join(def.Storage.State, "ssh", "host_ed25519") {
		t.Fatalf("HostKey = %q", cfg.SSHTUI.HostKey)
	}
}

func TestEffectiveConfigStateDoesNotCascade(t *testing.T) {
	cfg, _ := effectiveConfig(InitOptions{StateDir: "/srv/state"})
	def := config.Default()
	if cfg.Storage.State != "/srv/state" {
		t.Fatalf("State = %q", cfg.Storage.State)
	}
	if cfg.Storage.Profiles != def.Storage.Profiles {
		t.Fatalf("Profiles cascaded under --state: %q", cfg.Storage.Profiles)
	}
	if cfg.Storage.Models != def.Storage.Models {
		t.Fatalf("Models cascaded under --state: %q", cfg.Storage.Models)
	}
	if cfg.Storage.Cache != def.Storage.Cache {
		t.Fatalf("Cache cascaded under --state: %q", cfg.Storage.Cache)
	}
	if cfg.SSHTUI.HostKey != "/srv/state/ssh/host_ed25519" {
		t.Fatalf("HostKey should follow state: %q", cfg.SSHTUI.HostKey)
	}
}

func TestEffectiveConfigEachRoleOverridesItself(t *testing.T) {
	cfg, _ := effectiveConfig(InitOptions{
		StateDir: "/a/state",
		Profiles: "/b/profiles",
		Models:   "/c/models",
		Cache:    "/d/cache",
	})
	if cfg.Storage.State != "/a/state" ||
		cfg.Storage.Profiles != "/b/profiles" ||
		cfg.Storage.Models != "/c/models" ||
		cfg.Storage.Cache != "/d/cache" {
		t.Fatalf("storage = %+v", cfg.Storage)
	}
	if cfg.SSHTUI.HostKey != "/a/state/ssh/host_ed25519" {
		t.Fatalf("HostKey = %q", cfg.SSHTUI.HostKey)
	}
}

func TestEffectiveConfigInventoryOverrides(t *testing.T) {
	cfg, _ := effectiveConfig(InitOptions{
		MemoryMax:  "64g",
		MemswapMax: "128g",
		CPUsMax:    "16",
		Devices:    []string{"/dev/dri/renderD129", "/dev/dri/card0"},
	})
	if cfg.Inventory.MemoryMax != "64g" ||
		cfg.Inventory.MemswapMax != "128g" ||
		cfg.Inventory.CPUsMax != "16" {
		t.Fatalf("inventory ceilings = %+v", cfg.Inventory)
	}
	if len(cfg.Inventory.Devices) != 2 ||
		cfg.Inventory.Devices[0] != "/dev/dri/renderD129" ||
		cfg.Inventory.Devices[1] != "/dev/dri/card0" {
		t.Fatalf("inventory devices = %+v", cfg.Inventory.Devices)
	}
}

func TestEffectiveConfigInventoryDefaultsWhenUnset(t *testing.T) {
	cfg, _ := effectiveConfig(InitOptions{})
	def := config.Default()
	if cfg.Inventory.MemoryMax != def.Inventory.MemoryMax ||
		cfg.Inventory.MemswapMax != def.Inventory.MemswapMax ||
		cfg.Inventory.CPUsMax != def.Inventory.CPUsMax ||
		len(cfg.Inventory.Devices) != len(def.Inventory.Devices) {
		t.Fatalf("inventory drifted from defaults: %+v vs %+v", cfg.Inventory, def.Inventory)
	}
}

func TestInitRejectsRelativeDevicePath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
	state, profiles, models, cache := tempRoles(t, dir)

	err := Init(InitOptions{
		ConfigPath: cfgPath,
		StateDir:   state,
		Profiles:   profiles,
		Models:     models,
		Cache:      cache,
		Devices:    []string{"dri/renderD129"},
	})
	if err == nil {
		t.Fatal("expected init to reject relative device path")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(cfgPath); !os.IsNotExist(statErr) {
		t.Fatalf("config should not be written when inventory is invalid: %v", statErr)
	}
}

func TestInitRejectsInvalidCPUsMax(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
	state, profiles, models, cache := tempRoles(t, dir)

	err := Init(InitOptions{
		ConfigPath: cfgPath,
		StateDir:   state,
		Profiles:   profiles,
		Models:     models,
		Cache:      cache,
		CPUsMax:    "NaN",
	})
	if err == nil {
		t.Fatal("expected init to reject NaN cpus_max")
	}
	if !strings.Contains(err.Error(), "invalid cpus_max") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitRejectsInvalidMemoryMax(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
	state, profiles, models, cache := tempRoles(t, dir)

	err := Init(InitOptions{
		ConfigPath: cfgPath,
		StateDir:   state,
		Profiles:   profiles,
		Models:     models,
		Cache:      cache,
		MemoryMax:  "huge",
	})
	if err == nil {
		t.Fatal("expected init to reject invalid memory_max")
	}
	if !strings.Contains(err.Error(), "invalid memory_max") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitWritesFlatLayoutWithDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "etc", "unumd.toml")
	state, profiles, models, cache := tempRoles(t, dir)

	if err := Init(InitOptions{
		ConfigPath: cfgPath,
		ServerName: "lab",
		StateDir:   state,
		Profiles:   profiles,
		Models:     models,
		Cache:      cache,
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "lab" {
		t.Fatalf("ServerName = %q", cfg.ServerName)
	}
	if cfg.Storage.State != state || cfg.Storage.Profiles != profiles ||
		cfg.Storage.Models != models || cfg.Storage.Cache != cache {
		t.Fatalf("storage = %+v", cfg.Storage)
	}
	if cfg.SSHTUI.HostKey != filepath.Join(state, "ssh", "host_ed25519") {
		t.Fatalf("HostKey = %q", cfg.SSHTUI.HostKey)
	}

	for _, path := range []string{
		state, profiles, models, cache,
		filepath.Join(state, "ssh"),
		filepath.Join(state, "tokens"),
		filepath.Join(state, "logs"),
		filepath.Join(state, "ssh", "host_ed25519"),
		filepath.Join(profiles, "qwen3-small-cpu.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}

	profile, err := os.ReadFile(filepath.Join(profiles, "qwen3-small-cpu.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), models+":/models:ro") {
		t.Fatalf("profile does not use models dir %q: %s", models, profile)
	}

	key, err := os.ReadFile(filepath.Join(state, "ssh", "host_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParseRawPrivateKey(key); err != nil {
		t.Fatalf("host key is not a private key: %v", err)
	}
}

func TestInitFailsOnExistingConfigWithoutOverwrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
	state, profiles, models, cache := tempRoles(t, dir)

	if err := os.WriteFile(cfgPath, []byte("server_name = \"keep\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Init(InitOptions{
		ConfigPath: cfgPath,
		StateDir:   state,
		Profiles:   profiles,
		Models:     models,
		Cache:      cache,
		ServerName: "replace",
	})
	if err == nil {
		t.Fatal("expected error for existing config without --overwrite")
	}
	if !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("expected --overwrite hint, got: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "keep" {
		t.Fatalf("ServerName = %q; existing config must not be replaced", cfg.ServerName)
	}

	for _, path := range []string{state, profiles, models, cache} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("role dir %s should not exist after rejected init: %v", path, statErr)
		}
	}
}

func TestInitOverwriteReplacesConfigButPreservesHostKeyAndProfile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
	state, profiles, models, cache := tempRoles(t, dir)

	if err := os.WriteFile(cfgPath, []byte("server_name = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := InitOptions{
		ConfigPath: cfgPath,
		StateDir:   state,
		Profiles:   profiles,
		Models:     models,
		Cache:      cache,
		ServerName: "first",
		Overwrite:  true,
	}
	if err := Init(first); err != nil {
		t.Fatal(err)
	}

	hostKey, err := os.ReadFile(filepath.Join(state, "ssh", "host_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(profiles, "qwen3-small-cpu.yaml")
	profileBefore, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, []byte("# operator edits\n"+string(profileBefore)), 0o644); err != nil {
		t.Fatal(err)
	}
	profileAfterEdit, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}

	second := first
	second.ServerName = "second"
	if err := Init(second); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerName != "second" {
		t.Fatalf("ServerName = %q; --overwrite must replace config", cfg.ServerName)
	}

	hostKeyAfter, err := os.ReadFile(filepath.Join(state, "ssh", "host_ed25519"))
	if err != nil {
		t.Fatal(err)
	}
	if string(hostKey) != string(hostKeyAfter) {
		t.Fatal("host key must be preserved across --overwrite init")
	}

	profileAfter, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(profileAfter) != string(profileAfterEdit) {
		t.Fatal("operator-edited starter profile must be preserved across --overwrite init")
	}
}

func tempRoles(t *testing.T, root string) (state, profiles, models, cache string) {
	t.Helper()
	return filepath.Join(root, "state"),
		filepath.Join(root, "profiles"),
		filepath.Join(root, "models"),
		filepath.Join(root, "cache")
}
