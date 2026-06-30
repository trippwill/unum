package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/inference"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/setup"
	"github.com/trippwill/unum/internal/sshkeys"
	"github.com/trippwill/unum/internal/sshui"
	"github.com/trippwill/unum/internal/tokens"
	"github.com/trippwill/unum/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}

	switch args[0] {
	case "init":
		return runInit(args[1:])
	case "config":
		return runConfig(args[1:])
	case "profiles":
		return runProfiles(args[1:])
	case "status":
		return runStatus(args[1:])
	case "ssh":
		return runSSH(args[1:])
	case "tokens":
		return runTokens(args[1:])
	case "serve":
		return runServe(args[1:])
	case "version":
		fmt.Println(version.String())
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := setup.InitOptions{}
	fs.StringVar(&opts.ConfigPath, "config", config.DefaultPath, "config file path")
	fs.StringVar(&opts.ServerName, "server-name", "", "server name")
	fs.StringVar(&opts.StateDir, "state", "", "state directory")
	fs.StringVar(&opts.Profiles, "profiles", "", "profile directory")
	fs.StringVar(&opts.Models, "models", "", "model directory")
	fs.StringVar(&opts.Cache, "cache", "", "cache directory")
	fs.StringVar(&opts.MemoryMax, "memory-max", "", "machine memory ceiling, e.g. 32g")
	fs.StringVar(&opts.MemswapMax, "memswap-max", "", "machine memswap ceiling, e.g. 32g")
	fs.StringVar(&opts.CPUsMax, "cpus-max", "", "machine cpus ceiling, fractional cores")
	fs.Var(repeatableString{&opts.Devices}, "device", "register an absolute device path; repeat for multiple")
	fs.BoolVar(&opts.Overwrite, "overwrite", false, "overwrite existing config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("init takes no positional arguments")
	}
	return setup.Init(opts)
}

type repeatableString struct{ values *[]string }

func (r repeatableString) String() string {
	if r.values == nil {
		return ""
	}
	return strings.Join(*r.values, ",")
}

func (r repeatableString) Set(v string) error {
	if r.values == nil {
		return fmt.Errorf("nil destination")
	}
	*r.values = append(*r.values, v)
	return nil
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("config subcommand is required")
	}
	switch args[0] {
	case "get":
		return runConfigGet(args[1:])
	case "update":
		return runConfigUpdate(args[1:])
	default:
		return fmt.Errorf("unknown config subcommand %q", args[0])
	}
}

func runConfigGet(args []string) error {
	fs := flag.NewFlagSet("config get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("config get takes no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	data, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func runConfigUpdate(args []string) error {
	fs := flag.NewFlagSet("config update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := setup.UpdateOptions{}
	fs.StringVar(&opts.ConfigPath, "config", config.DefaultPath, "config file path")
	fs.StringVar(&opts.ServerName, "server-name", "", "server name")
	fs.StringVar(&opts.StateDir, "state", "", "state directory")
	fs.StringVar(&opts.Profiles, "profiles", "", "profile directory")
	fs.StringVar(&opts.Models, "models", "", "model directory")
	fs.StringVar(&opts.Cache, "cache", "", "cache directory")
	fs.StringVar(&opts.MemoryMax, "memory-max", "", "machine memory ceiling, e.g. 32g")
	fs.StringVar(&opts.MemswapMax, "memswap-max", "", "machine memswap ceiling, e.g. 32g")
	fs.StringVar(&opts.CPUsMax, "cpus-max", "", "machine cpus ceiling, fractional cores")
	fs.Var(repeatableString{&opts.Devices}, "device", "replace the registered device list with the given absolute paths; repeat for multiple")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("config update takes no positional arguments")
	}
	return setup.Update(opts)
}

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("status takes no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	status, err := service.New(cfg, version.Version).Status(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("Server: %s\n", status.ServerName)
	fmt.Printf("Version: %s\n", status.Version)
	fmt.Printf("Runtime: %s\n", status.RuntimeBackend)
	fmt.Printf("SSH: %s\n", status.SSHAddress)
	fmt.Printf("Inference: %s\n", status.InferenceEndpoint)
	fmt.Printf("Running: %s\n", status.RunningProfile)
	fmt.Printf("Operations: %s\n", status.Operations)
	return nil
}

func runProfiles(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("profiles subcommand is required")
	}
	switch args[0] {
	case "list":
		return runProfilesList(args[1:])
	case "validate":
		return runProfilesValidate(args[1:])
	default:
		return fmt.Errorf("unknown profiles subcommand %q", args[0])
	}
}

func runProfilesList(args []string) error {
	fs := flag.NewFlagSet("profiles list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("profiles list takes no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	profiles, err := service.New(cfg, version.Version).ListProfiles(context.Background())
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tVALID\tSTATE\tREASON")
	for _, p := range profiles {
		valid := "invalid"
		if p.Valid {
			valid = "valid"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID, valid, p.State, p.Reason)
	}
	return w.Flush()
}

func runProfilesValidate(args []string) error {
	fs := flag.NewFlagSet("profiles validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("profiles validate requires exactly one profile id")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	result, err := service.New(cfg, version.Version).ValidateProfile(context.Background(), fs.Arg(0))
	if err != nil {
		return err
	}
	if result.Valid {
		fmt.Println("valid")
		return nil
	}
	for _, validationErr := range result.Errors {
		fmt.Println(validationErr)
	}
	return fmt.Errorf("profile %q is invalid", fs.Arg(0))
}

func runTokens(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("tokens subcommand is required")
	}
	switch args[0] {
	case "create":
		return runTokensCreate(args[1:])
	case "list":
		return runTokensList(args[1:])
	case "revoke":
		return runTokensRevoke(args[1:])
	default:
		return fmt.Errorf("unknown tokens subcommand %q", args[0])
	}
}

func runTokensCreate(args []string) error {
	fs := flag.NewFlagSet("tokens create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	name := fs.String("name", "", "token name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("tokens create takes no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	created, err := service.New(cfg, version.Version).CreateInferenceToken(context.Background(), *name)
	if err != nil {
		return err
	}
	fmt.Printf("id: %s\nname: %s\nprefix: %s\ntoken: %s\n", created.ID, created.Name, created.Prefix, created.Raw)
	return nil
}

func runTokensList(args []string) error {
	fs := flag.NewFlagSet("tokens list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("tokens list takes no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	list, err := service.New(cfg, version.Version).ListInferenceTokens(context.Background())
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPREFIX\tSTATUS\tCREATED")
	for _, token := range list {
		status := "active"
		if token.Revoked {
			status = "revoked"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", token.ID, token.Name, token.Prefix, status, token.CreatedAt)
	}
	return w.Flush()
}

func runTokensRevoke(args []string) error {
	fs := flag.NewFlagSet("tokens revoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("tokens revoke requires exactly one token id")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	return service.New(cfg, version.Version).RevokeInferenceToken(context.Background(), fs.Arg(0))
}

func runSSH(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("ssh subcommand is required")
	}
	switch args[0] {
	case "add-authorized-keys":
		return runSSHAddAuthorizedKeys(args[1:])
	case "add-key":
		return runSSHAddKey(args[1:])
	case "list-keys":
		return runSSHListKeys(args[1:])
	case "revoke-key":
		return runSSHRevokeKey(args[1:])
	default:
		return fmt.Errorf("unknown ssh subcommand %q", args[0])
	}
}

func runSSHAddKey(args []string) error {
	fs := flag.NewFlagSet("ssh add-key", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	registry := fs.String("registry", "", "authorized clients registry")
	name := fs.String("name", "", "client name")
	role := fs.String("role", sshkeys.AdminRole, "client role")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("ssh add-key requires exactly one public key path")
	}
	key, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read public key %s: %w", fs.Arg(0), err)
	}
	registryPath, err := sshRegistryPath(*configPath, *registry)
	if err != nil {
		return err
	}
	client, err := (sshkeys.Store{Path: registryPath}).Add(*name, *role, key)
	if err != nil {
		return err
	}
	fmt.Println(client.ID)
	return nil
}

func runSSHAddAuthorizedKeys(args []string) error {
	fs := flag.NewFlagSet("ssh add-authorized-keys", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	registry := fs.String("registry", "", "authorized clients registry")
	name := fs.String("name", "", "client name prefix")
	role := fs.String("role", sshkeys.AdminRole, "client role")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("ssh add-authorized-keys requires exactly one authorized_keys path")
	}
	keys, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read authorized_keys %s: %w", fs.Arg(0), err)
	}
	registryPath, err := sshRegistryPath(*configPath, *registry)
	if err != nil {
		return err
	}
	clients, skipped, err := (sshkeys.Store{Path: registryPath}).AddAuthorizedKeys(*name, *role, keys)
	if err != nil {
		return err
	}
	for _, client := range clients {
		fmt.Println(client.ID)
	}
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "skipped already registered keys: %d\n", skipped)
	}
	return nil
}

func runSSHListKeys(args []string) error {
	fs := flag.NewFlagSet("ssh list-keys", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	registry := fs.String("registry", "", "authorized clients registry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("ssh list-keys takes no positional arguments")
	}
	registryPath, err := sshRegistryPath(*configPath, *registry)
	if err != nil {
		return err
	}
	reg, err := (sshkeys.Store{Path: registryPath}).Load()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tROLE\tSTATUS\tCREATED")
	for _, client := range reg.Clients {
		status := "active"
		if client.Revoked {
			status = "revoked"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", client.ID, client.Name, client.Role, status, client.CreatedAt.Format(timeLayout))
	}
	return w.Flush()
}

func runSSHRevokeKey(args []string) error {
	fs := flag.NewFlagSet("ssh revoke-key", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	registry := fs.String("registry", "", "authorized clients registry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("ssh revoke-key requires exactly one client id")
	}
	registryPath, err := sshRegistryPath(*configPath, *registry)
	if err != nil {
		return err
	}
	return (sshkeys.Store{Path: registryPath}).Revoke(fs.Arg(0))
}

func sshRegistryPath(configPath, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg.Storage.State, "ssh", "authorized-clients.json"), nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", config.DefaultPath, "config file path")
	check := fs.Bool("check", false, "load config and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("serve takes no positional arguments")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *check {
		fmt.Printf("config ok: %s\n", cfg.ServerName)
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, cfg, service.New(cfg, version.Version))
}

func serve(ctx context.Context, cfg config.Config, svc *service.Service) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := make(chan error, 2)
	count := 1
	go func() { errc <- sshui.Serve(ctx, cfg, svc) }()
	if cfg.Inference.Enabled {
		count++
		tokenStore := tokens.Store{Path: filepath.Join(cfg.Storage.State, "tokens", "inference-tokens.json")}
		go func() { errc <- inference.Serve(ctx, cfg.Inference, svc, tokenStore) }()
	}
	var firstErr error
	for range count {
		if err := <-errc; err != nil && firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	return firstErr
}

func usage() {
	fmt.Println(`unumd controls the unum trusted-server inference manager.

Usage:
  unumd init [--config PATH] [--state PATH] [--profiles PATH] [--models PATH] [--cache PATH] [--memory-max SIZE] [--memswap-max SIZE] [--cpus-max N] [--device PATH ...] [--server-name NAME] [--overwrite]
  unumd config update [--config PATH] [--state PATH] [--profiles PATH] [--models PATH] [--cache PATH] [--memory-max SIZE] [--memswap-max SIZE] [--cpus-max N] [--device PATH ...] [--server-name NAME]
  unumd config get [--config PATH]
  unumd profiles list [--config PATH]
  unumd profiles validate [--config PATH] ID
  unumd status [--config PATH]
  unumd ssh add-authorized-keys [--config PATH] --name NAME [--role admin] PATH
  unumd ssh add-key [--config PATH] --name NAME [--role admin] PATH
  unumd ssh list-keys [--config PATH]
  unumd ssh revoke-key [--config PATH] ID
  unumd tokens create [--config PATH] --name NAME
  unumd tokens list [--config PATH]
  unumd tokens revoke [--config PATH] ID
  unumd serve --config PATH --check
  unumd version
  unumd help`)
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
