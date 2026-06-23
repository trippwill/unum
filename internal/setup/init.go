package setup

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/pem"
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
}

func Init(opts InitOptions) error {
	cfg := config.Default()
	if opts.ConfigPath == "" {
		opts.ConfigPath = config.DefaultPath
	}
	if opts.ServerName != "" {
		cfg.ServerName = opts.ServerName
	}
	if opts.StateDir != "" {
		cfg.Storage.State = opts.StateDir
		cfg.Storage.Profiles = filepath.Join(opts.StateDir, "profiles")
		cfg.Storage.Models = filepath.Join(opts.StateDir, "models")
		cfg.SSHTUI.HostKey = filepath.Join(opts.StateDir, "ssh", "host_ed25519")
	}
	if opts.Profiles != "" {
		cfg.Storage.Profiles = opts.Profiles
	}
	if opts.Models != "" {
		cfg.Storage.Models = opts.Models
	}

	if err := mkdirAll(filepath.Dir(opts.ConfigPath), 0o755); err != nil {
		return err
	}
	for _, dir := range []struct {
		path string
		perm os.FileMode
	}{
		{cfg.Storage.State, 0o750},
		{cfg.Storage.Profiles, 0o750},
		{cfg.Storage.Models, 0o750},
		{filepath.Join(cfg.Storage.State, "ssh"), 0o700},
		{filepath.Join(cfg.Storage.State, "tokens"), 0o700},
		{filepath.Join(cfg.Storage.State, "logs"), 0o750},
	} {
		if err := mkdirAll(dir.path, dir.perm); err != nil {
			return err
		}
	}

	if err := writeConfigIfMissing(opts.ConfigPath, cfg); err != nil {
		return err
	}
	if err := writeHostKeyIfMissing(cfg.SSHTUI.HostKey); err != nil {
		return err
	}
	if err := writeProfileIfMissing(filepath.Join(cfg.Storage.Profiles, "qwen3-small-cpu.yaml"), cfg); err != nil {
		return err
	}
	return nil
}

func mkdirAll(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	return nil
}

func writeConfigIfMissing(path string, cfg config.Config) error {
	data, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeFileIfMissing(path, append(data, '\n'), 0o644)
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

func writeProfileIfMissing(path string, cfg config.Config) error {
	profile := bytes.ReplaceAll(starterProfile, []byte(starterProfileDefaultModelsDir), []byte(cfg.Storage.Models))
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
