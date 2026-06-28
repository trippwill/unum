# unum

[![CI](https://github.com/trippwill/unum/actions/workflows/ci.yml/badge.svg)](https://github.com/trippwill/unum/actions/workflows/ci.yml)

Trusted-server local inference manager.

Unum lets a trusted remote terminal control LLM services running on a dedicated
Linux server without exposing container runtime details.

## Current shape

- rootful Fedora Linux + Podman
- one daemon: `unumd`
- SSH TUI control plane
- OpenAI-compatible `/openai/v1/*` inference endpoint with bearer-token auth
- TOML config and profiles
- JSON state for SSH keys and inference tokens

The first planned release tag is **v0.1.0**.

## Development

Install [mise](https://mise.jdx.dev/), then:

```bash
mise install
mise run test
mise run build
```

Useful tasks:

```bash
mise run fmt
mise run vet
mise run precommit
mise run integration-smoke
```

Every commit should be buildable, have passing tests, use Conventional Commits,
and get a rubber duck review first.

## Operations

See [`docs/operations.md`](docs/operations.md) for rootful Fedora+Podman install,
systemd setup, config/profile examples, SSH access, and inference token smoke
checks.

## Product vision

See [`product/vision.md`](product/vision.md). The original release framing in
[`product/unum-v0-product-brief.md`](product/unum-v0-product-brief.md) is
retained as a historical document.
