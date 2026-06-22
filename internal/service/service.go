package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/profile"
)

type Service struct {
	cfg     config.Config
	version string
}

type Status struct {
	ServerName        string
	Version           string
	RuntimeBackend    string
	SSHAddress        string
	InferenceEndpoint string
	ActiveProfile     string
	Operations        string
}

type ProfileSummary struct {
	ID     string
	Name   string
	Valid  bool
	State  string
	Reason string
}

type InstanceSummary struct {
	ID        string
	ProfileID string
	Runtime   string
	State     string
	Health    string
	Endpoint  string
}

type OperationSummary struct {
	ID      string
	Target  string
	Phase   string
	State   string
	Message string
}

type InferenceTokenSummary struct {
	ID        string
	Name      string
	Prefix    string
	Revoked   bool
	CreatedAt string
}

func New(cfg config.Config, version string) *Service {
	return &Service{cfg: cfg, version: version}
}

func (s *Service) Status(context.Context) (Status, error) {
	return Status{
		ServerName:        s.cfg.ServerName,
		Version:           s.version,
		RuntimeBackend:    s.cfg.Runtime.Backend,
		SSHAddress:        s.cfg.SSHTUI.Address,
		InferenceEndpoint: inferenceEndpoint(s.cfg.Inference),
		ActiveProfile:     s.cfg.Inference.ActiveProfile,
		Operations:        "idle",
	}, nil
}

func (s *Service) ListProfiles(context.Context) ([]ProfileSummary, error) {
	summaries, err := profile.LoadDir(s.cfg.Storage.Profiles)
	if err != nil {
		return nil, err
	}
	profiles := make([]ProfileSummary, 0, len(summaries))
	for _, summary := range summaries {
		reason := ""
		if len(summary.Validation.Errors) > 0 {
			reason = summary.Validation.Errors[0]
		}
		profiles = append(profiles, ProfileSummary{
			ID:     summary.ID,
			Name:   summary.Name,
			Valid:  summary.Validation.Valid,
			State:  "stopped",
			Reason: reason,
		})
	}
	return profiles, nil
}

func (s *Service) ValidateProfile(ctx context.Context, id string) (profile.ValidationResult, error) {
	profiles, err := profile.LoadDir(s.cfg.Storage.Profiles)
	if err != nil {
		return profile.ValidationResult{}, err
	}
	for _, summary := range profiles {
		if summary.ID == id {
			return summary.Validation, nil
		}
	}
	return profile.ValidationResult{}, fmt.Errorf("profile %q not found", id)
}

func (s *Service) ListInstances(context.Context) ([]InstanceSummary, error) {
	return []InstanceSummary{}, nil
}

func (s *Service) ListOperations(context.Context) ([]OperationSummary, error) {
	return []OperationSummary{}, nil
}

func (s *Service) ListInferenceTokens(context.Context) ([]InferenceTokenSummary, error) {
	return []InferenceTokenSummary{}, nil
}

func inferenceEndpoint(cfg config.InferenceConfig) string {
	if !cfg.Enabled || cfg.Address == "" {
		return ""
	}
	scheme := "https"
	if cfg.DevInsecureHTTP {
		scheme = "http"
	}
	host := cfg.Address
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	base := cfg.BasePath
	if base == "" {
		base = "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, strings.TrimRight(base, "/"))
}
