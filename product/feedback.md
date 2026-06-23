# Product feedback inbox

Use this file for product-owner feedback that must survive the conversation until it is promoted to an issue, backlog item, ADR, or implementation plan.

## Pending triage

### Docker backend for viable v0

Feedback: "Docker backend is required for a viable v0. Keep runtime backend seams and plan Docker adapter work instead of deleting WithRuntimeBackend/runtimeBackend."
Type: constraint
Impact: release scope
Decision: track
Action: Turn into v0 Docker backend planning/backlog item.

### Init config flags

Feedback: "init should expose more config options as flags, especially paths and ports."
Type: feature
Impact: release behavior
Decision: track
Action: Add `unumd init` flags for SSH address/host key, inference address/base path/TLS/dev HTTP, profile/model/state paths, and other v0 deployment basics.

### Init existing-config behavior

Feedback: "init should fail when the config file already exists, with an explicit --overwrite escape hatch if needed."
Type: decision
Impact: release behavior
Decision: track
Action: Change init idempotence semantics and update tests/docs.

### Runtime health probe

Feedback: "Decide whether to remove unused Podman Probe/RuntimeInfo or wire it into real runtime health before release."
Type: feature
Impact: quality
Decision: track
Action: Decide whether runtime health belongs in v0 status; remove or wire `Probe` accordingly.

### SSH LastSeenAt

Feedback: "Remove LastSeenAt until auth auditing exists, or implement real last-seen tracking before release."
Type: feature
Impact: quality
Decision: track
Action: Decide whether v0 needs auth last-seen auditing; otherwise delete `LastSeenAt`.

### TUI dimensions

Feedback: "Keep width/height until viewport/layout behavior proves they are unnecessary."
Type: constraint
Impact: quality
Decision: track
Action: Preserve dimensions while implementing correct TUI viewport/layout behavior.

## Promoted with follow-up work

### Profile format and runtime shape

Feedback: "Reconsider custom profile TOML versus Compose file format, Podman kube YAML, a subset, or multiple supported formats. Decision must cover replacing real scripts for vLLM on Arc B60 and ComfyUI/Docker workloads, including devices, mounts, env, shm, memory, entrypoint, command, privileged/security options, host networking, model metadata, health, and active inference endpoint."
Type: decision
Impact: release scope
Decision: promoted to ADR 0002
Action: Turn the Compose-compatible profile format decision into implementation issues/backlog items.

### Multi-endpoint and multi-service profiles

Feedback: "Multi-endpoint profiles are required for personal v0 use cases, e.g. one profile running an SGLang diffusion server plus a small OpenAI-compatible LLM/frontend endpoint. v0 will need multi-service/multi-container profile support."
Type: constraint
Impact: release scope
Decision: promoted to ADR 0002
Action: Plan validation, operations, instances, logs, and endpoint display for multi-container profiles.

### Serving model and profile-owned ports

Feedback: "v0 USP is Unum serving one profile at a time, but inference APIs may not need one fixed port. Different ports are desirable for remote agents; better to fail when a coding model is not running than route to a wrong active configuration."
Type: decision
Impact: release behavior
Decision: promoted to ADR 0002
Action: Plan endpoint routing/display docs and implementation from the ADR.
