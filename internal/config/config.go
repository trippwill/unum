package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const DefaultPath = "/etc/unum/unumd.toml"

type Config struct {
	ServerName string          `toml:"server_name"`
	SSHTUI     SSHTUIConfig    `toml:"ssh_tui"`
	Inference  InferenceConfig `toml:"inference"`
	Runtime    RuntimeConfig   `toml:"runtime"`
	Storage    StorageConfig   `toml:"storage"`
	Machine    MachineConfig   `toml:"machine"`
	Logs       LogsConfig      `toml:"logs"`
}

type SSHTUIConfig struct {
	Enabled bool   `toml:"enabled"`
	Address string `toml:"address"`
	HostKey string `toml:"host_key"`
}

type InferenceConfig struct {
	Enabled         bool   `toml:"enabled"`
	Address         string `toml:"address"`
	BasePath        string `toml:"base_path"`
	TLSCert         string `toml:"tls_cert"`
	TLSKey          string `toml:"tls_key"`
	DevInsecureHTTP bool   `toml:"dev_insecure_http"`
}

type RuntimeConfig struct {
	Backend string `toml:"backend"`
}

type StorageConfig struct {
	State    string `toml:"state"`
	Profiles string `toml:"profiles"`
	Models   string `toml:"models"`
	Cache    string `toml:"cache"`
}

type MachineConfig struct {
	MemoryMax  string   `toml:"memory_max"`
	MemswapMax string   `toml:"memswap_max"`
	CPUsMax    string   `toml:"cpus_max"`
	Devices    []string `toml:"devices"`
}

type LogsConfig struct {
	RetainDays int `toml:"retain_days"`
}

func Default() Config {
	return Config{
		ServerName: "unum",
		SSHTUI: SSHTUIConfig{
			Enabled: true,
			Address: ":2222",
			HostKey: "/var/lib/unum/ssh/host_ed25519",
		},
		Inference: InferenceConfig{
			Enabled:         true,
			Address:         "127.0.0.1:8770",
			BasePath:        "/openai/v1",
			DevInsecureHTTP: true,
		},
		Runtime: RuntimeConfig{
			Backend: "podman",
		},
		Storage: StorageConfig{
			State:    "/var/lib/unum",
			Profiles: "/var/lib/unum/profiles",
			Models:   "/var/lib/unum/models",
			Cache:    "/var/lib/unum/cache",
		},
		Machine: MachineConfig{
			MemoryMax:  "32g",
			MemswapMax: "32g",
			CPUsMax:    "0",
		},
		Logs: LogsConfig{
			RetainDays: 14,
		},
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := Default()
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func Marshal(cfg Config) ([]byte, error) {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return data, nil
}
