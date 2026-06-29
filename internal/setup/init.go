package setup

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	"github.com/trippwill/unum/internal/config"
)

const starterProfileDefaultModelsDir = "/var/lib/unum/models"

//go:embed starter_profiles/qwen3-small-cpu.yaml
var starterProfile []byte

type InitOptions struct {
	ConfigPath string
	ServerName string
	StateDir   string
	Profiles   string
	Models     string
	Cache      string
	Overwrite  bool
}

func Init(opts InitOptions) error {
	cfg, configPath := effectiveConfig(opts)

	if !opts.Overwrite {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("config %s already exists; pass --overwrite to replace it", configPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", configPath, err)
		}
	}

	if err := mkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	for _, dir := range []struct {
		path string
		perm os.FileMode
	}{
		{cfg.Storage.State, 0o750},
		{cfg.Storage.Profiles, 0o750},
		{cfg.Storage.Models, 0o750},
		{cfg.Storage.Cache, 0o750},
		{filepath.Join(cfg.Storage.State, "ssh"), 0o700},
		{filepath.Join(cfg.Storage.State, "tokens"), 0o700},
		{filepath.Join(cfg.Storage.State, "logs"), 0o750},
	} {
		if err := mkdirAll(dir.path, dir.perm); err != nil {
			return err
		}
	}

	if err := writeConfig(configPath, cfg, opts.Overwrite); err != nil {
		return err
	}
	if err := writeHostKeyIfMissing(cfg.SSHTUI.HostKey); err != nil {
		return err
	}
	if err := writeProfileIfMissing(filepath.Join(cfg.Storage.Profiles, "qwen3-small-cpu.yaml"), cfg.Storage.Models); err != nil {
		return err
	}
	return nil
}

// effectiveConfig applies opts on top of config.Default() without touching the
// filesystem. Each StorageConfig role is set independently from the matching
// opts field; no role cascades from another. Returns the resolved config and
// the resolved config file path.
func effectiveConfig(opts InitOptions) (config.Config, string) {
	cfg := config.Default()
	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = config.DefaultPath
	}
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
	cfg.SSHTUI.HostKey = filepath.Join(cfg.Storage.State, "ssh", "host_ed25519")
	return cfg, configPath
}

func mkdirAll(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	return nil
}

func writeConfig(path string, cfg config.Config, overwrite bool) error {
	data, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	payload := append(data, '\n')
	if overwrite {
		if err := os.WriteFile(path, payload, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("config %s already exists; pass --overwrite to replace it", path)
		}
		return fmt.Errorf("write %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(payload); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeHostKeyIfMissing(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat host key %s: %w", path, err)
	}

	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ssh host key: %w", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(key, "unumd")
	if err != nil {
		return fmt.Errorf("marshal ssh host key: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		return fmt.Errorf("write ssh host key %s: %w", path, err)
	}
	return nil
}

func writeProfileIfMissing(path string, modelsDir string) error {
	profile := bytes.ReplaceAll(starterProfile, []byte(starterProfileDefaultModelsDir), []byte(modelsDir))
	return writeFileIfMissing(path, profile, 0o644)
}

func writeFileIfMissing(path string, data []byte, perm os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
