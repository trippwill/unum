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

	"gopkg.in/yaml.v3"
)

const MaxInferenceMemoryBytes = 32 * 1024 * 1024 * 1024

type Profile struct {
	Version  string             `yaml:"version,omitempty"`
	Services map[string]Service `yaml:"services"`
	Unum     UnumMetadata       `yaml:"x-unum"`
	ID       string             `yaml:"-"`
	Name     string             `yaml:"-"`
	Path     string             `yaml:"-"`
}

type Service struct {
	Image         string            `yaml:"image"`
	ContainerName string            `yaml:"container_name"`
	NetworkMode   string            `yaml:"network_mode"`
	Devices       []string          `yaml:"devices"`
	Volumes       []string          `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
	MemLimit      string            `yaml:"mem_limit"`
	MemswapLimit  string            `yaml:"memswap_limit"`
	ShmSize       string            `yaml:"shm_size"`
	SecurityOpt   []string          `yaml:"security_opt"`
	Entrypoint    string            `yaml:"entrypoint"`
	Command       StringList        `yaml:"command"`
}

type UnumMetadata struct {
	ID              string              `yaml:"id"`
	Name            string              `yaml:"name"`
	Endpoints       map[string]Endpoint `yaml:"endpoints"`
	Models          []string            `yaml:"models"`
	RequiredDevices []string            `yaml:"required_devices"`
}

type Endpoint struct {
	Service string `yaml:"service"`
	URL     string `yaml:"url"`
	Health  string `yaml:"health"`
}

type StringList []string

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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("parse profile %s: %w", path, err)
	}
	p.ID = p.Unum.ID
	p.Name = p.Unum.Name
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
		if entry.IsDir() || !isProfileFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		p, err := LoadFile(path)
		if err != nil {
			summaries = append(summaries, Summary{ID: trimProfileExt(entry.Name()), Path: path, Validation: ValidationResult{Errors: []string{err.Error()}}})
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
	if strings.TrimSpace(p.Unum.ID) == "" {
		errs = append(errs, "x-unum.id is required")
	}
	if strings.TrimSpace(p.Unum.Name) == "" {
		errs = append(errs, "x-unum.name is required")
	}
	if len(p.Services) == 0 {
		errs = append(errs, "services must contain at least one service")
	}
	if len(p.Unum.Endpoints) == 0 {
		errs = append(errs, "x-unum.endpoints must contain at least one endpoint")
	}

	serviceNames := sortedKeys(p.Services)
	for _, name := range serviceNames {
		svc := p.Services[name]
		if strings.TrimSpace(svc.Image) == "" {
			errs = append(errs, "services."+name+".image is required")
		}
		validateMemory := memoryValidator(&errs)
		memory, hasMemory := validateMemory("services."+name+".mem_limit", svc.MemLimit)
		swap, hasSwap := validateMemory("services."+name+".memswap_limit", svc.MemswapLimit)
		if hasMemory && hasSwap && swap < memory {
			errs = append(errs, "services."+name+".memswap_limit cannot be less than services."+name+".mem_limit")
		}
		for _, volume := range svc.Volumes {
			host, container, _, err := ParseVolume(volume)
			if err != nil {
				errs = append(errs, "services."+name+".volumes: "+err.Error())
				continue
			}
			if !filepath.IsAbs(host) {
				errs = append(errs, "services."+name+".volumes host must be absolute: "+volume)
			}
			if !filepath.IsAbs(container) {
				errs = append(errs, "services."+name+".volumes container must be absolute: "+volume)
			}
		}
		for _, device := range svc.Devices {
			host, err := ParseDeviceHost(device)
			if err != nil {
				errs = append(errs, "services."+name+".devices: "+err.Error())
				continue
			}
			if !filepath.IsAbs(host) {
				errs = append(errs, "services."+name+".devices host must be absolute: "+device)
			}
		}
	}

	endpointNames := sortedKeys(p.Unum.Endpoints)
	for _, name := range endpointNames {
		endpoint := p.Unum.Endpoints[name]
		if strings.TrimSpace(endpoint.URL) == "" {
			errs = append(errs, "x-unum.endpoints."+name+".url is required")
		}
		if strings.TrimSpace(endpoint.Health) == "" {
			errs = append(errs, "x-unum.endpoints."+name+".health is required")
		}
		if endpoint.Service == "" {
			if len(p.Services) != 1 {
				errs = append(errs, "x-unum.endpoints."+name+".service is required for multi-service profiles")
			}
			continue
		}
		if _, ok := p.Services[endpoint.Service]; !ok {
			errs = append(errs, "x-unum.endpoints."+name+".service references unknown service "+endpoint.Service)
		}
	}

	for _, model := range p.Unum.Models {
		if strings.TrimSpace(model) == "" {
			errs = append(errs, "x-unum.models cannot contain blank entries")
			continue
		}
		if !filepath.IsAbs(model) {
			errs = append(errs, "x-unum.models entries must be absolute")
			continue
		}
		if _, err := os.Stat(model); err != nil {
			errs = append(errs, "x-unum.models entry is not accessible: "+err.Error())
		}
	}
	for _, device := range p.Unum.RequiredDevices {
		if strings.TrimSpace(device) == "" {
			errs = append(errs, "x-unum.required_devices cannot contain blank entries")
			continue
		}
		if !filepath.IsAbs(device) {
			errs = append(errs, "x-unum.required_devices entries must be absolute")
			continue
		}
		if _, err := os.Stat(device); err != nil {
			errs = append(errs, "x-unum.required_devices entry is not accessible: "+err.Error())
		}
	}

	sort.Strings(errs)
	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

func (p Profile) SingleService() (string, Service, error) {
	if len(p.Services) == 0 {
		return "", Service{}, fmt.Errorf("profile has no services")
	}
	if len(p.Services) != 1 {
		return "", Service{}, fmt.Errorf("profile has %d services; multi-service start is not implemented", len(p.Services))
	}
	for name, svc := range p.Services {
		return name, svc, nil
	}
	panic("unreachable")
}

func (p Profile) EndpointURL() string {
	if endpoint, ok := p.Unum.Endpoints["openai"]; ok {
		return endpoint.URL
	}
	names := sortedKeys(p.Unum.Endpoints)
	if len(names) == 0 {
		return ""
	}
	return p.Unum.Endpoints[names[0]].URL
}

func ParseVolume(value string) (string, string, string, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", fmt.Errorf("volume must be host:container[:mode]: %q", value)
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", "", fmt.Errorf("volume host and container are required: %q", value)
	}
	if len(parts) == 3 && strings.TrimSpace(parts[2]) == "" {
		return "", "", "", fmt.Errorf("volume mode cannot be blank: %q", value)
	}
	return parts[0], parts[1], strings.Join(parts[2:], ":"), nil
}

func ParseDeviceHost(value string) (string, error) {
	parts := strings.Split(value, ":")
	if len(parts) == 0 || len(parts) > 3 || strings.TrimSpace(parts[0]) == "" {
		return "", fmt.Errorf("device must be host[:container[:permissions]]: %q", value)
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("device entries cannot contain blank parts: %q", value)
		}
	}
	return parts[0], nil
}

func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value == "" {
			*s = nil
			return nil
		}
		*s = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var values []string
		if err := value.Decode(&values); err != nil {
			return err
		}
		*s = values
		return nil
	default:
		return fmt.Errorf("must be a string or list of strings")
	}
}

func memoryValidator(errs *[]string) func(string, string) (int64, bool) {
	return func(field, value string) (int64, bool) {
		if strings.TrimSpace(value) == "" {
			return 0, false
		}
		parsed, err := parseMemory(value)
		if err != nil {
			*errs = append(*errs, field+": "+err.Error())
			return 0, false
		}
		if parsed > MaxInferenceMemoryBytes {
			*errs = append(*errs, field+" exceeds 32g v0 limit")
		}
		return parsed, true
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isProfileFile(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".yaml" || ext == ".yml"
}

func trimProfileExt(name string) string {
	return strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
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
