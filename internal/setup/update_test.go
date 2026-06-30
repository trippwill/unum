package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trippwill/unum/internal/config"
)

func TestUpdateRejectsMissingConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")

	err := Update(UpdateOptions{ConfigPath: cfgPath, MemoryMax: "64g"})
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestUpdateMutatesOnlySpecifiedFields(t *testing.T) {
	cfgPath, before := seedConfig(t)
	if err := Update(UpdateOptions{
		ConfigPath: cfgPath,
		MemoryMax:  "64g",
	}); err != nil {
		t.Fatal(err)
	}

	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.Machine.MemoryMax != "64g" {
		t.Fatalf("MemoryMax not updated: %q", after.Machine.MemoryMax)
	}
	if after.ServerName != before.ServerName {
		t.Fatalf("ServerName changed: %q -> %q", before.ServerName, after.ServerName)
	}
	if after.Storage != before.Storage {
		t.Fatalf("storage changed: %+v -> %+v", before.Storage, after.Storage)
	}
	if after.Machine.MemswapMax != before.Machine.MemswapMax ||
		after.Machine.CPUsMax != before.Machine.CPUsMax {
		t.Fatalf("other machine fields changed: %+v -> %+v", before.Machine, after.Machine)
	}
}

func TestUpdateReplacesDeviceList(t *testing.T) {
	cfgPath, _ := seedConfig(t)
	if err := Update(UpdateOptions{
		ConfigPath: cfgPath,
		Devices:    []string{"/dev/dri/renderD129", "/dev/dri/card0"},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Machine.Devices) != 2 ||
		after.Machine.Devices[0] != "/dev/dri/renderD129" ||
		after.Machine.Devices[1] != "/dev/dri/card0" {
		t.Fatalf("devices = %+v", after.Machine.Devices)
	}

	if err := Update(UpdateOptions{
		ConfigPath: cfgPath,
		Devices:    []string{"/dev/dri/renderD130"},
	}); err != nil {
		t.Fatal(err)
	}
	after, err = config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Machine.Devices) != 1 || after.Machine.Devices[0] != "/dev/dri/renderD130" {
		t.Fatalf("devices after second update = %+v", after.Machine.Devices)
	}
}

func TestUpdateValidatesMachineBeforeWrite(t *testing.T) {
	cfgPath, before := seedConfig(t)
	beforeBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	err = Update(UpdateOptions{ConfigPath: cfgPath, MemoryMax: "huge"})
	if err == nil {
		t.Fatal("expected error for invalid memory_max")
	}
	if !strings.Contains(err.Error(), "invalid memory_max") {
		t.Fatalf("unexpected error: %v", err)
	}

	afterBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) != string(afterBytes) {
		t.Fatal("config file was modified despite validation failure")
	}
	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.ServerName != before.ServerName {
		t.Fatalf("ServerName changed after rejected update: %q -> %q", before.ServerName, after.ServerName)
	}
}

func TestUpdatePreservesHostKeyWhenStateChanges(t *testing.T) {
	cfgPath, before := seedConfig(t)
	if err := Update(UpdateOptions{
		ConfigPath: cfgPath,
		StateDir:   "/srv/new-state",
	}); err != nil {
		t.Fatal(err)
	}
	after, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if after.Storage.State != "/srv/new-state" {
		t.Fatalf("State = %q", after.Storage.State)
	}
	if after.SSHTUI.HostKey != before.SSHTUI.HostKey {
		t.Fatalf("HostKey changed automatically: %q -> %q", before.SSHTUI.HostKey, after.SSHTUI.HostKey)
	}
}

func seedConfig(t *testing.T) (string, config.Config) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "unumd.toml")
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
	return cfgPath, cfg
}
