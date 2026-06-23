# Product feedback inbox

Use this file for product-owner feedback that must survive the conversation until it is promoted to an issue, backlog item, ADR, or implementation plan.

## Promoted to issues

### Docker backend for viable v0

Feedback: "Docker backend is required for a viable v0. Keep runtime backend seams and plan Docker adapter work instead of deleting WithRuntimeBackend/runtimeBackend."
Type: constraint
Impact: release scope
Decision: promoted to issue
Action: [#2 Add Docker runtime backend](https://github.com/trippwill/unum/issues/2)

### Init config flags

Feedback: "init should expose more config options as flags, especially paths and ports."
Type: feature
Impact: release behavior
Decision: promoted to issue
Action: [#3 Expand unumd init configuration flags and overwrite policy](https://github.com/trippwill/unum/issues/3)

### Init existing-config behavior

Feedback: "init should fail when the config file already exists, with an explicit --overwrite escape hatch if needed."
Type: decision
Impact: release behavior
Decision: promoted to issue
Action: [#3 Expand unumd init configuration flags and overwrite policy](https://github.com/trippwill/unum/issues/3)

### Runtime health probe

Feedback: "Decide whether to remove unused Podman Probe/RuntimeInfo or wire it into real runtime health before release."
Type: feature
Impact: quality
Decision: promoted to issue
Action: [#5 Decide and implement runtime health status](https://github.com/trippwill/unum/issues/5)

### SSH LastSeenAt

Feedback: "Remove LastSeenAt until auth auditing exists, or implement real last-seen tracking before release."
Type: feature
Impact: quality
Decision: promoted to issue
Action: [#6 Decide SSH LastSeenAt and TUI viewport cleanup](https://github.com/trippwill/unum/issues/6)

### TUI dimensions

Feedback: "Keep width/height until viewport/layout behavior proves they are unnecessary."
Type: constraint
Impact: quality
Decision: promoted to issue
Action: [#6 Decide SSH LastSeenAt and TUI viewport cleanup](https://github.com/trippwill/unum/issues/6)

### Profile format and runtime shape

Feedback: "Reconsider custom profile TOML versus Compose file format, Podman kube YAML, a subset, or multiple supported formats. Decision must cover replacing real scripts for vLLM on Arc B60 and ComfyUI/Docker workloads, including devices, mounts, env, shm, memory, entrypoint, command, privileged/security options, host networking, model metadata, health, and active inference endpoint."
Type: decision
Impact: release scope
Decision: promoted to ADR 0002 and issue
Action: [#1 Implement Compose-compatible multi-service profiles](https://github.com/trippwill/unum/issues/1)

### Multi-endpoint and multi-service profiles

Feedback: "Multi-endpoint profiles are required for personal v0 use cases, e.g. one profile running an SGLang diffusion server plus a small OpenAI-compatible LLM/frontend endpoint. v0 will need multi-service/multi-container profile support."
Type: constraint
Impact: release scope
Decision: promoted to ADR 0002 and issues
Action: [#1 Implement Compose-compatible multi-service profiles](https://github.com/trippwill/unum/issues/1), [#4 Implement profile-owned endpoint display and routing model](https://github.com/trippwill/unum/issues/4)

### Serving model and profile-owned ports

Feedback: "v0 USP is Unum serving one profile at a time, but inference APIs may not need one fixed port. Different ports are desirable for remote agents; better to fail when a coding model is not running than route to a wrong active configuration."
Type: decision
Impact: release behavior
Decision: promoted to ADR 0002 and issue
Action: [#4 Implement profile-owned endpoint display and routing model](https://github.com/trippwill/unum/issues/4)
