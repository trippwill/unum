package profile

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const MaxInferenceMemoryBytes = 32 * 1024 * 1024 * 1024

type Profile struct {
	ID          string           `toml:"id"`
	Name        string           `toml:"name"`
	Description string           `toml:"description"`
	Runtime     Runtime          `toml:"runtime"`
	Image       Image            `toml:"image"`
	Model       Model            `toml:"model"`
	Server      Server           `toml:"server"`
	Resources   Resources        `toml:"resources"`
	Mounts      map[string]Mount `toml:"mounts"`
	Container   Container        `toml:"container"`
	Path        string           `toml:"-"`
}

type Runtime struct {
	Backend string `toml:"backend"`
}

type Image struct {
	Ref string `toml:"ref"`
}

type Model struct {
	Path string `toml:"path"`
}

type Server struct {
	Kind       string `toml:"kind"`
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	HealthPath string `toml:"health_path"`
}

type Resources struct {
	Memory     string `toml:"memory"`
	MemorySwap string `toml:"memory_swap"`
	Threads    int    `toml:"threads"`
}

type Mount struct {
	Host      string `toml:"host"`
	Container string `toml:"container"`
	ReadOnly  bool   `toml:"read_only"`
}

type Container struct {
	Network string   `toml:"network"`
	Devices []string `toml:"devices"`
	Args    []string `toml:"args"`
}

type Summary struct {
	ID         string
	Name       string
	Path       string
	Validation ValidationResult
}

type ValidationResult struct {
	Valid  bool
	Errors []string
}

func LoadFile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile %s: %w", path, err)
	}
	var p Profile
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", path, err)
	}
	p.Path = path
	return p, nil
}

func LoadDir(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Summary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profile directory %s: %w", dir, err)
	}

	var summaries []Summary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		p, err := LoadFile(path)
		if err != nil {
			summaries = append(summaries, Summary{ID: strings.TrimSuffix(entry.Name(), ".toml"), Path: path, Validation: ValidationResult{Errors: []string{err.Error()}}})
			continue
		}
		summaries = append(summaries, Summary{
			ID:         p.ID,
			Name:       p.Name,
			Path:       path,
			Validation: Validate(p),
		})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func Find(dir, id string) (Profile, ValidationResult, error) {
	summaries, err := LoadDir(dir)
	if err != nil {
		return Profile{}, ValidationResult{}, err
	}
	for _, summary := range summaries {
		if summary.ID != id {
			continue
		}
		if !summary.Validation.Valid {
			return Profile{}, summary.Validation, nil
		}
		p, err := LoadFile(summary.Path)
		if err != nil {
			return Profile{}, ValidationResult{Errors: []string{err.Error()}}, nil
		}
		return p, summary.Validation, nil
	}
	return Profile{}, ValidationResult{}, fmt.Errorf("profile %q not found", id)
}

func Validate(p Profile) ValidationResult {
	var errs []string
	required := map[string]string{
		"id":                 p.ID,
		"runtime.backend":    p.Runtime.Backend,
		"image.ref":          p.Image.Ref,
		"model.path":         p.Model.Path,
		"server.kind":        p.Server.Kind,
		"server.host":        p.Server.Host,
		"server.health_path": p.Server.HealthPath,
		"container.network":  p.Container.Network,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, field+" is required")
		}
	}
	if p.Runtime.Backend != "" && p.Runtime.Backend != "podman" {
		errs = append(errs, "runtime.backend must be podman")
	}
	if p.Server.Kind != "" && p.Server.Kind != "openai-compatible" {
		errs = append(errs, "server.kind must be openai-compatible")
	}
	if p.Server.Port <= 0 || p.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}
	if p.Resources.Threads < 0 {
		errs = append(errs, "resources.threads cannot be negative")
	}
	memoryValues := map[string]int64{}
	validateMemory := func(field, value string) {
		if value == "" {
			return
		}
		bytes, err := parseMemory(value)
		if err != nil {
			errs = append(errs, field+": "+err.Error())
			return
		}
		if bytes > MaxInferenceMemoryBytes {
			errs = append(errs, field+" exceeds 32g v0 limit")
		}
		memoryValues[field] = bytes
	}
	validateMemory("resources.memory", p.Resources.Memory)
	validateMemory("resources.memory_swap", p.Resources.MemorySwap)
	if memory, ok := memoryValues["resources.memory"]; ok {
		if swap, ok := memoryValues["resources.memory_swap"]; ok && swap < memory {
			errs = append(errs, "resources.memory_swap cannot be less than resources.memory")
		}
	}
	if p.Model.Path != "" {
		if _, err := os.Stat(p.Model.Path); err != nil {
			errs = append(errs, "model.path is not accessible: "+err.Error())
		}
	}
	for name, mount := range p.Mounts {
		if mount.Host == "" || mount.Container == "" {
			errs = append(errs, "mounts."+name+" host and container are required")
		}
		if mount.Host != "" && !filepath.IsAbs(mount.Host) {
			errs = append(errs, "mounts."+name+".host must be absolute")
		}
		if mount.Container != "" && !filepath.IsAbs(mount.Container) {
			errs = append(errs, "mounts."+name+".container must be absolute")
		}
	}
	for _, device := range p.Container.Devices {
		if strings.TrimSpace(device) == "" {
			errs = append(errs, "container.devices cannot contain blank entries")
		} else if !filepath.IsAbs(device) {
			errs = append(errs, "container.devices entries must be absolute")
		}
	}
	sort.Strings(errs)
	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

func parseMemory(value string) (int64, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, fmt.Errorf("memory value is empty")
	}
	unit := value[len(value)-1]
	multiplier := int64(1)
	number := value
	switch unit {
	case 'k':
		multiplier = 1024
		number = value[:len(value)-1]
	case 'm':
		multiplier = 1024 * 1024
		number = value[:len(value)-1]
	case 'g':
		multiplier = 1024 * 1024 * 1024
		number = value[:len(value)-1]
	}
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid memory value %q", value)
	}
	if n > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("memory value %q is too large", value)
	}
	return n * multiplier, nil
}
