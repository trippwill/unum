# Copilot instructions for Unum

## Commands

- Install toolchain: `mise install`
- Format Go: `mise run fmt`
- Check formatting: `mise run fmt-check`
- Vet: `mise run vet`
- Test all packages: `mise run test`
- Run one package: `mise exec -- go test ./internal/sshkeys`
- Run one test: `mise exec -- go test ./internal/sshkeys -run TestStoreAddsAuthorizedKeysFile`
- Race tests: `mise exec -- go test -race ./...`
- Build daemon: `mise run build`
- Full local gate: `mise run precommit`
- CI-equivalent gate: `mise run ci`

## Architecture

Unum is a rootful Fedora + Podman daemon for trusted-server local inference management. The single production binary is `cmd/unumd`.

- `cmd/unumd` owns CLI parsing and command wiring only.
- `internal/config` owns strict TOML config loading and defaults.
- `internal/setup` owns `unumd init`, root-owned state/config creation, SSH host key generation, and starter profile creation.
- `internal/profile` owns profile TOML loading and validation.
- `internal/service` is the central use-case layer used by CLI, SSH TUI, and inference code.
- `internal/runtime/podman` is the only container backend; it shells directly to `podman` with fixed argv, never through a shell.
- `internal/sshui` serves the Wish/Bubble Tea SSH TUI and uses the service layer only.
- `internal/inference` serves the OpenAI-compatible `/openai/v1/*` reverse proxy with bearer-token auth.
- `internal/sshkeys` and `internal/tokens` store JSON registries under the configured state directory.

Default rootful paths are `/etc/unum/unumd.toml` and `/var/lib/unum`, but hostnames, IPs, model paths, TLS paths, memory limits, and device mappings must stay configurable.

## Project conventions

- Prefer `mise run <task>` over raw Go commands, except for targeted `mise exec -- go test ...` checks.
- Use Conventional Commits.
- Every commit must build and pass tests.
- Get rubber-duck review before committing.
- Get security review for security-sensitive changes.
- Do not push without explicit user sign-off.
- Use `/docs` only for user-facing docs; keep ADRs, briefs, feedback, and other product docs under `/product`.
- Add an ADR only when a decision affects architecture boundaries, major dependencies, protocols, storage/deployment models, durable security/operational constraints, deferred obvious alternatives, or release scope.
- Keep clean architecture at package boundaries; avoid future-only interfaces.
- Apply Ponytail/YAGNI inside boundaries: stdlib/native first, minimal dependencies, no speculative backends.
- Treat product-owner feedback as durable input: capture it, classify it, and map it to active release goals before acting.

## Security and behavior conventions

- SSH control-plane auth uses Unum's own registered public-key registry, not system SSH users.
- `ssh add-key` is strict: exactly one public key.
- `ssh add-authorized-keys` imports all non-comment keys, skips duplicates, and rejects OpenSSH options and certificate keys because v0 does not enforce their restrictions.
- Inference tokens are shown once, stored as hashes, and identified later by metadata/prefix only.
- The inference proxy validates Unum bearer tokens but strips `Authorization` and `Proxy-Authorization` before forwarding upstream.
- `dev_insecure_http = true` is only allowed on loopback inference addresses; wildcard binds like `:8770` must be rejected.
- Profile validation enforces the 32 GB v0 inference memory limit and explicit absolute device paths.
- JSON registries are whole-file writes with `ponytail:` comments; add file locking only if concurrent admins matter.
- In-memory operation/instance state is intentional for v0; persist it only when daemon restart recovery matters.
