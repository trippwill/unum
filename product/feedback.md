# Product feedback inbox

Use this file for product-owner feedback that must survive the conversation until it is promoted to an issue, backlog item, ADR, or implementation plan.

## Tracked for planning

### Developer branch update helper

Feedback: "I'd like a developer convenience for pulling a branch and updating to simplify user testing"
Type: feature
Impact: onboarding
Decision: do now
Action: Add a small `mise run dev-update -- BRANCH` helper for clean-worktree branch pull, CI gate, install, init, enable, and service restart on a test server.

### Configurable validation and defaults

Feedback: "default max memory should be configuration. probably others"
Type: constraint
Impact: release behavior
Decision: track
Action: Before the next profile-validation slice, decide which hard-coded profile limits/defaults belong in daemon config, starting with max memory.

### Profile templates as reusable bases

Feedback: "eventually users are going to want to have profile templates as a reusable base"
Type: feature
Impact: future
Decision: defer
Action: Track profile templates as an onboarding/reuse feature; do not add a template abstraction until a real reusable base exists.

### CLI and TUI profile template initialization

Feedback: "the cli and tui should be able to list profile templates, and initilize a new (inactive) profile file from a template"
Type: feature
Impact: onboarding
Decision: track
Action: Plan a later profile-template slice after Compose profile loading settles: list templates in CLI/TUI and create inactive profile files from a selected template.

### Profile descriptions in list views

Feedback: "profiles should support descriptions for display in the tui and cli list commands"
Type: feature
Impact: onboarding
Decision: track
Action: Add `x-unum.description` to the profile metadata model and include descriptions in CLI/TUI profile list views in the next profile-display slice.

### Built-in Hugging Face repository downloader

Feedback: "future feature: built-in hf repo downloader"
Type: feature
Impact: future
Decision: defer
Action: Track as a future model-management feature; keep v0 starter smoke tests manual until profile/runtime basics are stable.

### TUI logs missing during smoke test

Feedback: "container is started. that's good. appears in tui. that's good. no logs in tui."
Follow-up: "also the long instance ids are terrible ux"
Follow-up: "also ux: if we're going to prepend container names with unum_ we need to show that in the tui and cli otherwise users will look for qwen3-small-cpu container and not find it"
Type: bug
Impact: v0 behavior
Decision: track
Action: Investigate TUI log loading/selection against a started profile instance before release, show the actual runtime container name in CLI/TUI, and shorten or hide full container IDs in instance/log messages.

### Hugging Face CLI model download workflow

Feedback: "the model download didn't go well, I generally expect to use hf cli to download a model repo, but I don't know what llama.cpp expects for a GGUF"
Follow-up: "I'm going to download as a repo, and then symlink into the repo from models/ to the actual gguf file. v0 note: we should assume/support full repo downloads and not just GGUFs b/c dynamic online quantization has to be supporteed"
Type: feature
Impact: v0 behavior
Decision: track
Action: Document an HF CLI repo-download workflow for smoke tests and ensure future downloader/model handling supports full model repositories, not only standalone GGUF files, because dynamic online quantization needs repo contents.

### File and repository model path validation

Feedback: "we're going to need a solution for repo and single file based models, maybe the compose style config handles it already, but validation may need to be updates?"
Type: question
Impact: v0 behavior
Decision: track
Action: Decide whether `x-unum.models` should distinguish files, directories, or generic paths; update validation and examples so both standalone GGUF files and full model repositories are supported.

### Model directory permissions during smoke test

Feedback: "permissions are too tight on models for my user to cd or ls it"
Follow-up: "it's actually the parent /var/lib/unum that had the tight permissions"
Type: bug
Impact: onboarding
Decision: track
Action: Revisit init-created `/var/lib/unum` and model directory permissions or documented operator group ownership so admins can traverse, inspect, and populate models without weakening rootful runtime assumptions.

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
