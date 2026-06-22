package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/trippwill/unum/internal/config"
	"github.com/trippwill/unum/internal/setup"
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
  unumd serve --config PATH --check
  unumd version
  unumd help`)
}
