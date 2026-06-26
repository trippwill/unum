package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"strings"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/profile"
	"github.com/trippwill/unum/internal/runtime/podman"
	"github.com/trippwill/unum/internal/tokens"
)

type Service struct {
	cfg     config.Config
	version string
	runtime runtimeBackend
	mu      sync.Mutex
	nextOp  int
	// ponytail: in-memory operation/instance state; persist when daemon restart recovery matters.
	operations []OperationSummary
	instances  map[string]InstanceSummary
	events     []Event
}

type Option func(*Service)

type runtimeBackend interface {
	EnsureImage(context.Context, string) error
	Create(context.Context, profile.Profile) (podman.ContainerID, error)
	Start(context.Context, podman.ContainerID) error
	Stop(context.Context, podman.ContainerID) error
	Remove(context.Context, podman.ContainerID) error
	Inspect(context.Context, podman.ContainerID) (podman.ContainerStatus, error)
	Logs(context.Context, podman.ContainerID, podman.LogOptions) (<-chan podman.LogLine, error)
}

type Status struct {
	ServerName        string
	Version           string
	RuntimeBackend    string
	SSHAddress        string
	InferenceEndpoint string
	RunningProfile    string
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
	Name      string
	ProfileID string
	Runtime   string
	State     string
	Health    string
	Endpoint  string
	StartedAt string
}

type OperationSummary struct {
	ID      string
	Target  string
	Phase   string
	State   string
	Message string
}

type LogLine struct {
	InstanceID string
	Text       string
	Err        error
}

type LogOptions struct {
	Tail   int
	Follow bool
}

type Event struct {
	OperationID string
	Target      string
	Phase       string
	State       string
	Message     string
	At          time.Time
}

type InferenceTokenSummary struct {
	ID        string
	Name      string
	Prefix    string
	Revoked   bool
	CreatedAt string
}

type CreatedInferenceToken struct {
	ID     string
	Name   string
	Prefix string
	Raw    string
}

func WithRuntimeBackend(backend runtimeBackend) Option {
	return func(s *Service) {
		s.runtime = backend
	}
}

func New(cfg config.Config, version string, opts ...Option) *Service {
	s := &Service{
		cfg:       cfg,
		version:   version,
		runtime:   podman.New(),
		instances: map[string]InstanceSummary{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Status(context.Context) (Status, error) {
	s.mu.Lock()
	runningProfile := s.runningProfileLocked()
	s.mu.Unlock()
	return Status{
		ServerName:        s.cfg.ServerName,
		Version:           s.version,
		RuntimeBackend:    s.cfg.Runtime.Backend,
		SSHAddress:        s.cfg.SSHTUI.Address,
		InferenceEndpoint: inferenceEndpoint(s.cfg.Inference),
		RunningProfile:    runningProfile,
		Operations:        s.operationState(),
	}, nil
}

func (s *Service) ListProfiles(context.Context) ([]ProfileSummary, error) {
	summaries, err := profile.LoadDir(s.cfg.Storage.Profiles, s.profileValidationOptions())
	if err != nil {
		return nil, err
	}
	profiles := make([]ProfileSummary, 0, len(summaries))
	for _, summary := range summaries {
		reason := ""
		if len(summary.Validation.Errors) > 0 {
			reason = summary.Validation.Errors[0]
		}
		state := "stopped"
		s.mu.Lock()
		if instance, ok := s.instances[summary.ID]; ok {
			state = instance.State
		}
		s.mu.Unlock()
		profiles = append(profiles, ProfileSummary{
			ID:     summary.ID,
			Name:   summary.Name,
			Valid:  summary.Validation.Valid,
			State:  state,
			Reason: reason,
		})
	}
	return profiles, nil
}

func (s *Service) ValidateProfile(ctx context.Context, id string) (profile.ValidationResult, error) {
	profiles, err := profile.LoadDir(s.cfg.Storage.Profiles, s.profileValidationOptions())
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

func (s *Service) StartProfile(ctx context.Context, id string) (OperationSummary, error) {
	op := s.beginOperation(id, "start")
	p, validation, err := profile.Find(s.cfg.Storage.Profiles, id, s.profileValidationOptions())
	if err != nil {
		return s.failOperation(op.ID, "validating", err.Error()), err
	}
	if !validation.Valid {
		reason := "profile invalid"
		if len(validation.Errors) > 0 {
			reason = validation.Errors[0]
		}
		err := fmt.Errorf("profile %q is invalid: %s", id, reason)
		return s.failOperation(op.ID, "validating", err.Error()), err
	}
	_, svc, err := p.SingleService()
	if err != nil {
		return s.failOperation(op.ID, "validating", err.Error()), err
	}
	if running, ok := s.runningInstance(); ok {
		err := fmt.Errorf("profile %q is already running", running.ProfileID)
		if running.ProfileID != id {
			err = fmt.Errorf("profile %q is already running; stop it before starting %q", running.ProfileID, id)
		}
		return s.failOperation(op.ID, "checking state", err.Error()), err
	}
	s.updateOperation(op.ID, "checking image", "running", svc.Image)
	if err := s.runtime.EnsureImage(ctx, svc.Image); err != nil {
		return s.failOperation(op.ID, "checking image", err.Error()), err
	}
	s.updateOperation(op.ID, "creating container", "running", id)
	containerID, err := s.runtime.Create(ctx, p)
	if err != nil {
		return s.failOperation(op.ID, "creating container", err.Error()), err
	}
	s.updateOperation(op.ID, "starting container", "running", string(containerID))
	if err := s.runtime.Start(ctx, containerID); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if cleanupErr := s.runtime.Remove(cleanupCtx, containerID); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove failed container %s: %w", containerID, cleanupErr))
		}
		return s.failOperation(op.ID, "starting container", err.Error()), err
	}
	s.updateOperation(op.ID, "waiting for health", "running", string(containerID))
	status, err := s.runtime.Inspect(ctx, containerID)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if stopErr := s.runtime.Stop(cleanupCtx, containerID); stopErr != nil {
			err = errors.Join(err, fmt.Errorf("stop failed container %s: %w", containerID, stopErr))
		}
		if cleanupErr := s.runtime.Remove(cleanupCtx, containerID); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("remove failed container %s: %w", containerID, cleanupErr))
		}
		return s.failOperation(op.ID, "waiting for health", err.Error()), err
	}
	s.mu.Lock()
	s.instances[id] = InstanceSummary{
		ID:        string(containerID),
		Name:      status.Name,
		ProfileID: id,
		Runtime:   s.cfg.Runtime.Backend,
		State:     status.State,
		Health:    status.Health,
		Endpoint:  p.EndpointURL(),
		StartedAt: formatTime(status.Started),
	}
	s.mu.Unlock()
	return s.succeedOperation(op.ID, "ready", string(containerID)), nil
}

func (s *Service) profileValidationOptions() profile.ValidationOptions {
	return profile.ValidationOptions{MaxMemory: s.cfg.Profiles.MaxMemory}
}

func (s *Service) runningInstance() (InstanceSummary, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, instance := range s.instances {
		if instance.State == "running" {
			return instance, true
		}
	}
	return InstanceSummary{}, false
}

func (s *Service) runningProfileLocked() string {
	running := ""
	for _, instance := range s.instances {
		if instance.State != "running" {
			continue
		}
		if running != "" && running != instance.ProfileID {
			return "multiple"
		}
		running = instance.ProfileID
	}
	return running
}

func (s *Service) StopProfile(ctx context.Context, id string) (OperationSummary, error) {
	op := s.beginOperation(id, "stop")
	s.mu.Lock()
	instance, ok := s.instances[id]
	s.mu.Unlock()
	if !ok {
		err := fmt.Errorf("profile %q has no running instance", id)
		return s.failOperation(op.ID, "stopping container", err.Error()), err
	}
	containerID := podman.ContainerID(instance.ID)
	s.updateOperation(op.ID, "stopping container", "running", instance.ID)
	if err := s.runtime.Stop(ctx, containerID); err != nil {
		return s.failOperation(op.ID, "stopping container", err.Error()), err
	}
	s.updateOperation(op.ID, "removing container", "running", instance.ID)
	if err := s.runtime.Remove(ctx, containerID); err != nil {
		return s.failOperation(op.ID, "removing container", err.Error()), err
	}
	s.mu.Lock()
	delete(s.instances, id)
	s.mu.Unlock()
	return s.succeedOperation(op.ID, "stopped", instance.ID), nil
}

func (s *Service) ListInstances(context.Context) ([]InstanceSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	instances := make([]InstanceSummary, 0, len(s.instances))
	for _, instance := range s.instances {
		instances = append(instances, instance)
	}
	return instances, nil
}

func (s *Service) TailLogs(ctx context.Context, instanceID string, lines int) ([]LogLine, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := s.StreamLogs(ctx, instanceID, LogOptions{Tail: lines})
	if err != nil {
		return nil, err
	}
	var out []LogLine
	for line := range stream {
		if line.Err != nil {
			return out, line.Err
		}
		out = append(out, line)
	}
	return out, nil
}

// StreamLogs streams until the runtime closes the log stream or ctx is cancelled.
func (s *Service) StreamLogs(ctx context.Context, instanceID string, opts LogOptions) (<-chan LogLine, error) {
	if instanceID == "" {
		return nil, fmt.Errorf("instance id is required")
	}
	if _, ok := s.findInstance(instanceID); !ok {
		return nil, fmt.Errorf("instance %q not found", instanceID)
	}
	lines, err := s.runtime.Logs(ctx, podman.ContainerID(instanceID), podman.LogOptions{Tail: opts.Tail, Follow: opts.Follow})
	if err != nil {
		return nil, err
	}
	out := make(chan LogLine, 128)
	go func() {
		defer close(out)
		for line := range lines {
			select {
			case out <- LogLine{InstanceID: instanceID, Text: line.Text, Err: line.Err}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *Service) ListOperations(context.Context) ([]OperationSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ops := append([]OperationSummary(nil), s.operations...)
	return ops, nil
}

func (s *Service) ListInferenceTokens(context.Context) ([]InferenceTokenSummary, error) {
	list, err := s.tokenStore().List()
	if err != nil {
		return nil, err
	}
	summaries := make([]InferenceTokenSummary, 0, len(list))
	for _, token := range list {
		summaries = append(summaries, InferenceTokenSummary{
			ID:        token.ID,
			Name:      token.Name,
			Prefix:    token.Prefix,
			Revoked:   token.Revoked,
			CreatedAt: token.CreatedAt.Format(time.RFC3339),
		})
	}
	return summaries, nil
}

func (s *Service) CreateInferenceToken(_ context.Context, name string) (CreatedInferenceToken, error) {
	created, err := s.tokenStore().Create(name)
	if err != nil {
		return CreatedInferenceToken{}, err
	}
	return CreatedInferenceToken{
		ID:     created.Token.ID,
		Name:   created.Token.Name,
		Prefix: created.Token.Prefix,
		Raw:    created.Raw,
	}, nil
}

func (s *Service) RevokeInferenceToken(_ context.Context, id string) error {
	return s.tokenStore().Revoke(id)
}

func (s *Service) WatchEvents(context.Context) (<-chan Event, error) {
	s.mu.Lock()
	events := append([]Event(nil), s.events...)
	s.mu.Unlock()
	ch := make(chan Event, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func (s *Service) beginOperation(target, phase string) OperationSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextOp++
	op := OperationSummary{ID: fmt.Sprintf("op_%d", s.nextOp), Target: target, Phase: phase, State: "running"}
	s.operations = append(s.operations, op)
	s.events = append(s.events, Event{OperationID: op.ID, Target: target, Phase: phase, State: "running", At: time.Now().UTC()})
	return op
}

func (s *Service) updateOperation(id, phase, state, message string) OperationSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.operations {
		if s.operations[i].ID == id {
			s.operations[i].Phase = phase
			s.operations[i].State = state
			s.operations[i].Message = message
			s.events = append(s.events, Event{OperationID: id, Target: s.operations[i].Target, Phase: phase, State: state, Message: message, At: time.Now().UTC()})
			return s.operations[i]
		}
	}
	return OperationSummary{}
}

func (s *Service) failOperation(id, phase, message string) OperationSummary {
	return s.updateOperation(id, phase, "failed", message)
}

func (s *Service) succeedOperation(id, phase, message string) OperationSummary {
	return s.updateOperation(id, phase, "succeeded", message)
}

func (s *Service) operationState() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.operations) - 1; i >= 0; i-- {
		if s.operations[i].State == "running" {
			return s.operations[i].Phase
		}
	}
	return "idle"
}

func (s *Service) findInstance(instanceID string) (InstanceSummary, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, instance := range s.instances {
		if instance.ID == instanceID {
			return instance, true
		}
	}
	return InstanceSummary{}, false
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (s *Service) tokenStore() tokens.Store {
	return tokens.Store{Path: filepath.Join(s.cfg.Storage.State, "tokens", "inference-tokens.json")}
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
