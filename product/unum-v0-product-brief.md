# Unum Product Brief

> **Historical document.** The canonical product direction now lives in [`vision.md`](vision.md). This brief is retained as the original framing for Core and `v0.1.0` and is not maintained as the source of truth.

## Product

**Unum** is a trusted-server local inference manager.

It lets a user control LLM services running on a dedicated Linux server from a trusted remote client, without assuming inference runs on the workstation and without exposing users to container runtime details.

The first UI is an **SSH TUI** served directly by `unumd`.

```text
trusted laptop terminal
  └── ssh unum
        ▼
unumd SSH TUI
        ▼
server-side profiles / containers / logs / tokens
        ▼
local LLM inference services
```

---

## Core premise

Most current local inference tools assume:

```text
my workstation runs the model
my UI controls localhost
my GPU is in the same machine I am using
```

Unum assumes:

```text
my trusted server runs the model
my trusted client controls the server
my workstation is only the operator interface
```

This is the main product distinction.

---

## Roadmap terminology

Use precise names:

- **Core** is the internal engineering proof gate. It proves Unum can operate one trusted-server inference profile end-to-end. It is not a release tag.
- **`v0.1.0`** is the first intended release tag. It follows Go/SemVer tagging conventions and should include the work required by Core plus the work needed for a marketable technical release.
- The product exists as planned releases, not internal engineering milestones.
- **`v0`** alone means only the pre-1.0 compatibility era. Do not use it as a roadmap bucket.

Core is complete. Use it as the proof baseline; plan new work against the `v0.1.0` release experience.

---

## Target user

### `v0.1.0` user

A technical user with:

- a Linux server;
- Podman installed;
- one or more local models;
- a desire to run inference services remotely;
- comfort using SSH;
- no desire to manually manage every `podman run` script.

This is initially for the `unum` server, but should be shaped so it can later become a general product.

---

## `v0.1.0` goals

### Primary goals

- Manage LLM serving profiles on a trusted Linux server.
- Start, stop, restart, and validate profiles.
- Stream logs from running inference containers.
- Show server/runtime/model/profile health.
- Generate and revoke OpenAI-compatible inference tokens.
- Hide Podman/container details behind Unum profiles.
- Provide a remote-first admin experience over SSH.

### Secondary goals

- Keep the backend abstraction clean enough to add Docker for `v0.1.0`.
- Keep profile model independent of Podman.
- Keep inference API compatible with OpenAI-style clients.
- Make container runtime support an implementation detail.

### Post-Core planning posture

Core proved the thin end-to-end loop. `v0.1.0` planning should now optimize for a nice, consistent, helpful operator experience:

- reduce setup, profile, token, and troubleshooting toil;
- make TUI, CLI, logs, validation, and inference errors say what the operator can do next;
- plan features as coherent flows, not isolated minimum proof slices;
- implement those flows in small, buildable slices;
- get the flow clear before building, then implement the smallest code that delivers it.

---

## Non-goals before `v0.1.0`

Do **not** build these yet unless a later release decision explicitly pulls them in:

- local desktop GUI;
- workstation-local inference;
- containerd backend;
- Kubernetes backend;
- multi-user hosted service;
- OAuth/OIDC;
- browser UI;
- automatic model discovery across Hugging Face;
- complex RBAC;
- public internet exposure;
- GPU autodetection beyond simple server capability reporting;
- automatic profile scheduling;
- arbitrary container orchestration.

---

## Product shape

### Components

```text
unumd
  ├── SSH TUI listener
  ├── inference HTTP listener
  ├── profile manager
  ├── runtime backend interface
  │     └── Podman backend
  ├── operation manager
  ├── event/log stream
  ├── token manager
  └── model/profile storage
```

### First UI

```text
ssh -p 2222 unum.internal
```

The first UI is a Bubble Tea / Wish TUI served by `unumd`.

The user does not install a graphical client. The client requirement is simply:

```text
ssh
```

---

## `v0.1.0` user experience

### First-time setup

On the server:

```bash
sudo unumd init
sudo unumd ssh add-key --name charles-thinkpad --role admin ~/.ssh/id_ed25519.pub
sudo systemctl enable --now unumd
```

From the laptop:

```bash
ssh -p 2222 unum.internal
```

Inside the TUI:

```text
Unum: unum

Runtime: Podman
Inference endpoint: https://unum.internal:8770/openai/v1
Active profile: qwen3-small-cpu
Running profiles: 1
Operations: idle
```

---

## `v0.1.0` screens

### 1. Dashboard

Shows:

- server name;
- daemon version;
- runtime backend;
- running profile;
- active inference profile;
- current operations;
- inference endpoint;
- basic CPU/RAM/device status.

Example:

```text
Unum Server
──────────────

Server:      unum
Runtime:     Podman
Backend:     CPU
Running:     qwen3-small-cpu
Inference:   https://unum.internal:8770/openai/v1
State:       Ready
```

### 2. Profiles

Shows configured profiles.

Actions:

- start;
- stop;
- restart;
- validate;
- view config;
- view logs.

A profile is Unum’s core unit of configuration:

```text
profile = image + model + launch args + resource limits + serving endpoint
```

Not:

```text
container = podman-specific object
```

### 3. Instances

Shows currently or recently running profile instances.

Useful fields:

- profile ID;
- runtime backend;
- container ID, for debugging only;
- state;
- started at;
- health;
- endpoint;
- restart count / failure reason.

### 4. Logs

Streams logs from the selected running instance.

Basic controls:

- tail latest;
- follow;
- pause;
- search/filter later;
- copy selected lines later.

### 5. Operations

Shows long-running tasks:

- pulling image;
- validating model path;
- creating container;
- starting container;
- waiting for health;
- stopping container.

States:

```text
queued
running
succeeded
failed
cancelled
```

### 6. Inference Tokens

Manage OpenAI-style bearer tokens.

Actions:

- create token;
- list token names/prefixes;
- revoke token.

Tokens have one simple scope:

```text
inference
```

Control-plane access is **not** granted by bearer tokens.

---

## Security model

### SSH TUI control plane

Control access uses SSH public key authentication.

Unum maintains its own allowed key registry:

```text
/var/lib/unum/ssh/authorized-clients.json
```

Do not implicitly allow every system SSH user to control Unum.

A Unum SSH client record:

```json
{
  "id": "sshclient_01JZ...",
  "name": "charles-thinkpad",
  "publicKey": "ssh-ed25519 AAAA...",
  "role": "admin",
  "revoked": false,
  "createdAt": "2026-06-22T18:00:00Z",
  "lastSeenAt": null
}
```

For `v0.1.0`, one role is enough:

```text
admin
```

Later:

```text
admin
operator
viewer
```

### Inference plane

Inference uses OpenAI-style bearer tokens:

```http
Authorization: Bearer unum_sk_...
```

Inference tokens can only call inference endpoints.

They cannot:

- start profiles;
- stop profiles;
- pull images;
- create more tokens;
- read control logs;
- alter daemon config.

Token storage:

```text
server stores token hash only
token is shown once
token prefix is kept for display
```

---

## Networking

### SSH TUI listener

```text
unum.internal:2222
```

Auth:

```text
SSH public key
Unum key registry
```

### Inference listener

```text
https://unum.internal:8770/openai/v1
```

Auth:

```text
OpenAI-style bearer token
```

### Optional local admin socket

For server-local tooling:

```text
/run/unum/unumd.sock
```

Control HTTP/mTLS can come later.

---

## Inference routing

`unumd` should proxy or expose a stable inference endpoint.

Recommended `v0.1.0` endpoint:

```text
https://unum.internal:8770/openai/v1
```

The running profile receives default traffic.

Later explicit profile routing can be added:

```text
/profiles/{profileId}/openai/v1/...
```

For `v0.1.0`:

```text
running profile only
```

If no profile is running:

```text
503 Service Unavailable
```

Do not let inference tokens auto-start stopped profiles in `v0.1.0`.

---

## Profile model

A profile file uses Compose-compatible YAML with Unum metadata in `x-unum`.

Example:

```yaml
services:
  qwen3-small-cpu:
    image: ghcr.io/ggml-org/llama.cpp:server
    container_name: unum-qwen3-small-cpu
    network_mode: host
    volumes:
      - /srv/unum/models:/models:ro
    mem_limit: 4g
    memswap_limit: 4g
    command: ["--model", "/models/Qwen_Qwen3-0.6B-Q4_K_M.gguf", "--host", "127.0.0.1", "--port", "18080"]

x-unum:
  id: qwen3-small-cpu
  name: Qwen3 Small CPU
  endpoints:
    openai:
      service: qwen3-small-cpu
      url: http://127.0.0.1:18080/v1
      health: /health
```

For the `unum` server, later XPU profiles can use the same structure with devices added.

---

## Runtime backend abstraction

Keep this internal interface small.

```go
type RuntimeBackend interface {
    Name() string
    Probe(ctx context.Context) (*RuntimeInfo, error)

    EnsureImage(ctx context.Context, image ImageRef) error
    Create(ctx context.Context, spec ContainerSpec) (ContainerID, error)
    Start(ctx context.Context, id ContainerID) error
    Stop(ctx context.Context, id ContainerID) error
    Remove(ctx context.Context, id ContainerID) error

    Inspect(ctx context.Context, id ContainerID) (*ContainerStatus, error)
    Logs(ctx context.Context, id ContainerID, opts LogOptions) (<-chan LogLine, error)
}
```

Core implementation target:

```text
PodmanBackend
```

`v0.1.0` implementation target:

```text
DockerBackend
```

Future:

```text
ContainerdBackend
KubernetesBackend
```

Do not leak Podman concepts into the profile model or TUI.

---

## Internal service layer

The TUI should talk to a Unum service interface, not Podman directly.

```go
type ControlService interface {
    Status(ctx context.Context) (*Status, error)

    ListProfiles(ctx context.Context) ([]ProfileSummary, error)
    GetProfile(ctx context.Context, id string) (*Profile, error)
    ValidateProfile(ctx context.Context, id string) (*ValidationResult, error)

    StartProfile(ctx context.Context, id string) (OperationID, error)
    StopProfile(ctx context.Context, id string) (OperationID, error)
    RestartProfile(ctx context.Context, id string) (OperationID, error)
    ListInstances(ctx context.Context) ([]InstanceSummary, error)
    TailLogs(ctx context.Context, instanceID string, lines int) ([]LogLine, error)
    StreamLogs(ctx context.Context, instanceID string, opts LogOptions) (<-chan LogLine, error)

    ListOperations(ctx context.Context) ([]OperationSummary, error)
    WatchEvents(ctx context.Context) (<-chan Event, error)

    ListInferenceTokens(ctx context.Context) ([]InferenceTokenSummary, error)
    CreateInferenceToken(ctx context.Context, name string) (*CreatedInferenceToken, error)
    RevokeInferenceToken(ctx context.Context, id string) error
}
```

This lets you later add:

```text
HTTP control API
native GUI
unumctl
```

without rewriting core logic.

---

## Storage layout

For rootful server daemon:

```text
/var/lib/unum/
  ├── state.db
  ├── profiles/
  │   ├── qwen3-small-cpu.yaml
  │   └── qwen3-coder-xpu.yaml
  ├── ssh/
  │   ├── host_ed25519
  │   └── authorized-clients.json
  ├── tokens/
  │   └── inference-tokens.json
  └── logs/
```

For a custom setup, profile files may also be sourced from:

```text
/srv/unum/profiles
```

or another explicit configured path.

---

## Packaging for `v0.1.0`

### Binary

Start with one binary:

```text
unumd
```

Optional symlink/subcommand behavior later:

```text
unumctl
```

### systemd service

```ini
[Unit]
Description=Unum local inference manager
After=network-online.target podman.service
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/unumd serve --config /etc/unum/unumd.toml
Restart=on-failure
User=root

[Install]
WantedBy=multi-user.target
```

Rootful is acceptable for `v0.1.0` if workloads and device mappings require it. Later rootless can be explored.

---

## Initial config

```toml
server_name = "unum"

[ssh_tui]
enabled = true
address = "192.168.31.10:2222"
host_key = "/var/lib/unum/ssh/host_ed25519"

[inference]
enabled = true
address = "192.168.31.10:8770"
base_path = "/openai/v1"

[runtime]
backend = "podman"

[storage]
state = "/var/lib/unum"
profiles = "/var/lib/unum/profiles"

[logs]
retain_days = 14
```

---

## `v0.1.0` command surface

Server-side:

```bash
unumd init
unumd serve
unumd ssh add-key --name charles-thinkpad --role admin ~/.ssh/id_ed25519.pub
unumd ssh list-keys
unumd ssh revoke-key <id>
unumd profiles validate <id>
```

Later:

```bash
unumctl status
unumctl profiles
unumctl start <profile>
unumctl stop <profile>
```

For `v0.1.0`, the TUI can be the main control surface.

---

## Completed Core engineering proof plan

This proof plan is retained as the Core completion record. New work belongs in `v0.1.0` release planning.

### Milestone 1: daemon skeleton

Deliver:

- config loading;
- state directory creation;
- structured logging;
- basic status service;
- SSH host key generation;
- Wish SSH listener;
- simple Bubble Tea dashboard.

Done when:

```bash
ssh -p 2222 unum.internal
```

opens a TUI showing daemon status.

### Milestone 2: profile registry

Deliver:

- load profile YAML files;
- list profiles in TUI;
- validate required fields;
- show validation errors.

Done when the TUI can show:

```text
qwen3-small-cpu    valid     stopped
qwen3-coder-xpu    invalid   model path missing
```

### Milestone 3: Podman backend

Deliver:

- connect to Podman;
- create/start/stop/remove container;
- map Unum profile to Podman create options;
- inspect status;
- stream logs.

Done when the TUI can start and stop one CPU profile.

### Milestone 4: operations/events

Deliver:

- operation IDs;
- lifecycle phases;
- event bus;
- TUI progress updates;
- failure messages.

Done when starting a profile shows:

```text
validating
checking image
creating container
starting container
waiting for health
ready
```

### Milestone 5: inference listener

Deliver:

- OpenAI-compatible reverse proxy;
- running profile routing;
- bearer token middleware;
- 503 when no profile is running.

Done when:

```bash
curl https://unum.internal:8770/openai/v1/chat/completions \
  -H "Authorization: Bearer unum_sk_..." \
  ...
```

routes to the active local service.

### Milestone 6: token management

Deliver:

- create inference token;
- show token once;
- hash tokens server-side;
- list token metadata;
- revoke token;
- TUI token screen.

Done when a user can create a token in the TUI and paste it into an editor.

---

## `v0.1.0` release intent

`v0.1.0` should be the first tagged release that is useful to a technical user outside the current dogfood setup.

Post-Core, "useful" means more than the loop working once: routine setup, profile operation, token use, logs, validation, and failure recovery should feel consistent and should reduce operator toil.

Include:

- the work required by Core;
- install and operations docs;
- clear rootful ownership and operator access behavior;
- profile descriptions in CLI and TUI lists;
- model repository and single-file model path UX;
- profile-owned endpoint display and routing;
- Compose-compatible multi-service profiles;
- accelerator/device validation for explicit hardware paths;
- Docker backend support.

Defer unless explicitly pulled in:

- built-in Hugging Face downloader;
- profile template wizard;
- browser or desktop UI;
- public internet exposure;
- OAuth/OIDC/RBAC.

---

## Key product decisions

### Decision 1

Unum is **server-first**, not workstation-first.

### Decision 2

The first UI is **SSH TUI**, not desktop GUI.

### Decision 3

`unumd` serves the SSH TUI itself for `v0.1.0`.

### Decision 4

Inference access uses OpenAI-style bearer tokens.

### Decision 5

Control access uses SSH public key auth for `v0.1.0`.

### Decision 6

mTLS control API is deferred until there is a native client or remote HTTP control need.

### Decision 7

Podman is the Core runtime backend.

### Decision 8

Docker/containerd support requires backend adapters, not profile model changes. Docker is a `v0.1.0` release-scope decision; containerd stays future.

---

## Sharp edges to accept

- Linux-only.
- Podman-only.
- Admin-only control role.
- Manual SSH key registration.
- Manual profile file creation.
- No automatic model download workflow initially.
- No complex GPU detection.
- No local desktop app.
- No Kubernetes.
- No public internet story.

These constraints are intentional. They protect the product from becoming a generic orchestration platform too early.

For `v0.1.0`, revisit only the Podman-only and hardware-profile sharp edges against the release intent. Keep the rest unless real users are blocked.

---

## North star

Unum is a **trusted local AI server console**.

It should feel like:

```text
I have a server with serious inference capability.
Unum lets me operate it cleanly from wherever I work.
```

Not:

```text
Here is another local chat app that happens to start containers.
```

That distinction should drive `v0.1.0`.
