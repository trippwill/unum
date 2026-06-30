package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/trippwill/unum/internal/config"
)

// UpdateOptions modifies fields of an existing config file. Only fields with
// non-zero values are applied; other config fields are preserved as loaded.
// Devices, when non-empty, replace the existing machine device list as a
// whole — there is no add/remove semantic.
type UpdateOptions struct {
	ConfigPath string
	ServerName string
	StateDir   string
	Profiles   string
	Models     string
	Cache      string
	MemoryMax  string
	MemswapMax string
	CPUsMax    string
	Devices    []string
}

// Update edits the config file at opts.ConfigPath (or config.DefaultPath) in
// place. It loads the existing config, applies any explicitly-set overrides,
// validates the resulting machine, and then atomically rewrites the file.
// Filesystem directories are not created; the operator manages those (re-run
// `unumd init --overwrite` for full reinitialization). Storage paths and host
// key are not auto-derived from each other on update.
func Update(opts UpdateOptions) error {
	path := opts.ConfigPath
	if path == "" {
		path = config.DefaultPath
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg = applyUpdateOverrides(cfg, opts)
	if err := validateMachine(cfg); err != nil {
		return err
	}
	return writeConfigAtomic(path, cfg)
}

func applyUpdateOverrides(cfg config.Config, opts UpdateOptions) config.Config {
	if opts.ServerName != "" {
		cfg.ServerName = opts.ServerName
	}
	if opts.StateDir != "" {
		cfg.Storage.State = opts.StateDir
	}
	if opts.Profiles != "" {
		cfg.Storage.Profiles = opts.Profiles
	}
	if opts.Models != "" {
		cfg.Storage.Models = opts.Models
	}
	if opts.Cache != "" {
		cfg.Storage.Cache = opts.Cache
	}
	if opts.MemoryMax != "" {
		cfg.Machine.MemoryMax = opts.MemoryMax
	}
	if opts.MemswapMax != "" {
		cfg.Machine.MemswapMax = opts.MemswapMax
	}
	if opts.CPUsMax != "" {
		cfg.Machine.CPUsMax = opts.CPUsMax
	}
	if len(opts.Devices) > 0 {
		cfg.Machine.Devices = append([]string(nil), opts.Devices...)
	}
	return cfg
}

func writeConfigAtomic(path string, cfg config.Config) error {
	data, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	payload := append(data, '\n')
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".unumd-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename %s -> %s: %w", tmpName, path, err)
	}
	return nil
}
