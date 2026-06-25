# Decision 0001: Core architecture

## Status

Accepted.

## Decision

Unum's Core engineering milestone is a rootful Fedora Linux + Podman service with one daemon binary, `unumd`.

`unumd` owns:

- an SSH TUI control plane;
- an OpenAI-compatible inference proxy with bearer-token auth;
- profile loading and validation;
- rootful Podman lifecycle operations;
- JSON-backed SSH key and inference token registries.

The implementation keeps boring clean boundaries:

- `cmd/unumd`: CLI wiring;
- `internal/config`: config parsing/defaults;
- `internal/profile`: profile loading/validation;
- `internal/runtime/podman`: direct `podman` argv calls;
- `internal/service`: use cases and state transitions;
- `internal/sshui`: SSH TUI;
- `internal/inference`: bearer auth and reverse proxy.

## Consequences

- Podman CLI is the Core backend. Go bindings can replace only `internal/runtime/podman` later if measured need appears.
- Rootless, browser UI, RBAC, OAuth/OIDC, and public internet exposure are deferred.
- Docker is not required to prove Core; `v0.1.0` release intent is tracked separately in the product brief.
- Hostnames, IPs, model paths, TLS paths, memory limits, and GPU devices stay configurable.
