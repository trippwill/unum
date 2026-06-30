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
	Volumes       Volumes           `yaml:"volumes"`
	Environment   map[string]string `yaml:"environment"`
	MemLimit      string            `yaml:"mem_limit"`
	MemswapLimit  string            `yaml:"memswap_limit"`
	ShmSize       string            `yaml:"shm_size"`
	OOMScoreAdj   *int              `yaml:"oom_score_adj"`
	SecurityOpt   []string          `yaml:"security_opt"`
	Entrypoint    string            `yaml:"entrypoint"`
	Command       StringList        `yaml:"command"`
	Cpus          string            `yaml:"cpus"`
}

type UnumMetadata struct {
	ID        string              `yaml:"id"`
	Name      string              `yaml:"name"`
	Endpoints map[string]Endpoint `yaml:"endpoints"`
}

type Endpoint struct {
	Service string `yaml:"service"`
	URL     string `yaml:"url"`
	Health  string `yaml:"health"`
}

type StringList []string

type Volumes []Volume

type Volume struct {
	Short    string
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

type Summary struct {
	ID         string
	Name       string
	Path       string
	Validation ValidationResult
}

type ValidationOptions struct {
	MemoryMax  string
	MemswapMax string
	CPUsMax    string
	Devices    []string
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

func LoadDir(dir string, opts ValidationOptions) ([]Summary, error) {
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
			Validation: Validate(p, opts),
		})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].ID < summaries[j].ID })
	return summaries, nil
}

func Find(dir, id string, opts ValidationOptions) (Profile, ValidationResult, error) {
	summaries, err := LoadDir(dir, opts)
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

func Validate(p Profile, opts ValidationOptions) ValidationResult {
	var errs []string
	memMax, memMaxLabel, hasMemMax := opts.parseMemoryCeiling("memory_max", opts.MemoryMax, &errs)
	swapMax, swapMaxLabel, hasSwapMax := opts.parseMemoryCeiling("memswap_max", opts.MemswapMax, &errs)
	cpuMax, cpuMaxLabel, hasCPUMax := opts.parseCPUCeiling(&errs)
	devices := deviceSet(opts.Devices)

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
		memField := "services." + name + ".mem_limit"
		swapField := "services." + name + ".memswap_limit"
		cpusField := "services." + name + ".cpus"
		memory, hasMemory := parseServiceMemory(memField, svc.MemLimit, memMax, memMaxLabel, hasMemMax, &errs)
		swap, hasSwap := parseServiceMemory(swapField, svc.MemswapLimit, swapMax, swapMaxLabel, hasSwapMax, &errs)
		if hasMemory && hasSwap && swap < memory {
			errs = append(errs, swapField+" cannot be less than "+memField)
		}
		parseServiceCPUs(cpusField, svc.Cpus, cpuMax, cpuMaxLabel, hasCPUMax, &errs)
		for _, volume := range svc.Volumes {
			for _, err := range validateVolume(volume) {
				errs = append(errs, "services."+name+".volumes: "+err)
			}
			if host := volumeHost(volume); host != "" && strings.HasPrefix(host, "/dev/") {
				if _, ok := devices[host]; !ok {
					errs = append(errs, "services."+name+".volumes: device path "+host+" is not in [machine].devices")
				}
			}
		}
		if svc.OOMScoreAdj != nil && (*svc.OOMScoreAdj < -1000 || *svc.OOMScoreAdj > 1000) {
			errs = append(errs, "services."+name+".oom_score_adj must be between -1000 and 1000")
		}
		for _, device := range svc.Devices {
			host, err := ParseDeviceHost(device)
			if err != nil {
				errs = append(errs, "services."+name+".devices: "+err.Error())
				continue
			}
			if !filepath.IsAbs(host) {
				errs = append(errs, "services."+name+".devices host must be absolute: "+device)
				continue
			}
			if _, ok := devices[host]; !ok {
				errs = append(errs, "services."+name+".devices: "+host+" is not in [machine].devices")
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

	sort.Strings(errs)
	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

func (opts ValidationOptions) parseMemoryCeiling(name, value string, errs *[]string) (int64, string, bool) {
	label := strings.TrimSpace(value)
	if label == "" {
		return 0, "", false
	}
	parsed, err := parseMemory(label)
	if err != nil {
		*errs = append(*errs, "profile validation "+name+": "+err.Error())
		return 0, "", false
	}
	return parsed, label, true
}

func (opts ValidationOptions) parseCPUCeiling(errs *[]string) (float64, string, bool) {
	label := strings.TrimSpace(opts.CPUsMax)
	if label == "" {
		return 0, "", false
	}
	parsed, err := parseCPUs(label)
	if err != nil {
		*errs = append(*errs, "profile validation cpus_max: "+err.Error())
		return 0, "", false
	}
	if parsed == 0 {
		return 0, "", false
	}
	return parsed, label, true
}

func parseServiceMemory(field, value string, ceiling int64, ceilingLabel string, hasCeiling bool, errs *[]string) (int64, bool) {
	if strings.TrimSpace(value) == "" {
		return 0, false
	}
	parsed, err := parseMemory(value)
	if err != nil {
		*errs = append(*errs, field+": "+err.Error())
		return 0, false
	}
	if hasCeiling && parsed > ceiling {
		*errs = append(*errs, field+" exceeds configured "+ceilingFieldFor(field)+" "+ceilingLabel)
	}
	return parsed, true
}

func parseServiceCPUs(field, value string, ceiling float64, ceilingLabel string, hasCeiling bool, errs *[]string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	parsed, err := parseCPUs(value)
	if err != nil {
		*errs = append(*errs, field+": "+err.Error())
		return
	}
	if parsed == 0 {
		*errs = append(*errs, field+" must be greater than 0")
		return
	}
	if hasCeiling && parsed > ceiling {
		*errs = append(*errs, field+" exceeds configured cpus_max "+ceilingLabel)
	}
}

func ceilingFieldFor(field string) string {
	if strings.HasSuffix(field, ".memswap_limit") {
		return "memswap_max"
	}
	return "memory_max"
}

func deviceSet(devices []string) map[string]struct{} {
	set := make(map[string]struct{}, len(devices))
	for _, d := range devices {
		set[d] = struct{}{}
	}
	return set
}

func volumeHost(v Volume) string {
	if v.Short != "" {
		host, _, _, err := ParseVolume(v.Short)
		if err != nil {
			return ""
		}
		return host
	}
	return v.Source
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

func validateVolume(volume Volume) []string {
	if volume.Short != "" {
		host, container, _, err := ParseVolume(volume.Short)
		if err != nil {
			return []string{err.Error()}
		}
		var errs []string
		if !filepath.IsAbs(host) {
			errs = append(errs, "host must be absolute: "+volume.Short)
		}
		if !filepath.IsAbs(container) {
			errs = append(errs, "container must be absolute: "+volume.Short)
		}
		return errs
	}
	var errs []string
	if strings.TrimSpace(volume.Type) != "bind" {
		errs = append(errs, "long form type must be bind")
	}
	if strings.TrimSpace(volume.Source) == "" {
		errs = append(errs, "long form source is required")
	} else if !filepath.IsAbs(volume.Source) {
		errs = append(errs, "long form source must be absolute: "+volume.Source)
	}
	if strings.TrimSpace(volume.Target) == "" {
		errs = append(errs, "long form target is required")
	} else if !filepath.IsAbs(volume.Target) {
		errs = append(errs, "long form target must be absolute: "+volume.Target)
	}
	return errs
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

func (s *Volumes) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("must be a list")
	}
	var volumes Volumes
	for _, item := range value.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			volumes = append(volumes, Volume{Short: item.Value})
		case yaml.MappingNode:
			volume, err := decodeLongVolume(item)
			if err != nil {
				return err
			}
			volumes = append(volumes, volume)
		default:
			return fmt.Errorf("volume must be a string or mapping")
		}
	}
	*s = volumes
	return nil
}

func decodeLongVolume(node *yaml.Node) (Volume, error) {
	allowed := map[string]bool{"type": true, "source": true, "target": true, "read_only": true}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if !allowed[key] {
			return Volume{}, fmt.Errorf("unknown volume field %q", key)
		}
	}
	var raw struct {
		Type     string `yaml:"type"`
		Source   string `yaml:"source"`
		Target   string `yaml:"target"`
		ReadOnly bool   `yaml:"read_only"`
	}
	if err := node.Decode(&raw); err != nil {
		return Volume{}, err
	}
	return Volume{Type: raw.Type, Source: raw.Source, Target: raw.Target, ReadOnly: raw.ReadOnly}, nil
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

func parseCPUs(value string) (float64, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, fmt.Errorf("cpus value is empty")
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpus value %q", value)
	}
	if n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, fmt.Errorf("invalid cpus value %q", value)
	}
	return n, nil
}
