package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/service"
	"github.com/trippwill/unum/internal/setup"
	"github.com/trippwill/unum/internal/sshkeys"
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
	case "profiles":
		return runProfiles(args[1:])
	case "status":
		return runStatus(args[1:])
	case "ssh":
		return runSSH(args[1:])
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("init takes no positional arguments")
	}
	return setup.Init(opts)
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
	fmt.Printf("Active: %s\n", status.ActiveProfile)
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

func runSSH(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("ssh subcommand is required")
	}
	switch args[0] {
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
	registry := fs.String("registry", sshkeys.DefaultRegistryPath, "authorized clients registry")
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
	client, err := (sshkeys.Store{Path: *registry}).Add(*name, *role, key)
	if err != nil {
		return err
	}
	fmt.Println(client.ID)
	return nil
}

func runSSHListKeys(args []string) error {
	fs := flag.NewFlagSet("ssh list-keys", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registry := fs.String("registry", sshkeys.DefaultRegistryPath, "authorized clients registry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("ssh list-keys takes no positional arguments")
	}
	reg, err := (sshkeys.Store{Path: *registry}).Load()
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
	registry := fs.String("registry", sshkeys.DefaultRegistryPath, "authorized clients registry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("ssh revoke-key requires exactly one client id")
	}
	return (sshkeys.Store{Path: *registry}).Revoke(fs.Arg(0))
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
	return fmt.Errorf("serve is not implemented yet")
}

func usage() {
	fmt.Println(`unumd controls the unum trusted-server inference manager.

Usage:
  unumd init [--config PATH] [--state PATH] [--server-name NAME]
  unumd profiles list [--config PATH]
  unumd profiles validate [--config PATH] ID
  unumd status [--config PATH]
  unumd ssh add-key --name NAME [--role admin] PATH
  unumd ssh list-keys
  unumd ssh revoke-key ID
  unumd serve --config PATH --check
  unumd version
  unumd help`)
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
