# Unum product vision

This is the canonical product vision for Unum. New work should map back to one of the four pillars below. Architecture decisions remain captured in `product/decisions/`; the original brief at `product/unum-v0-product-brief.md` is retained as historical context only.

## Vision

> Unum turns a trusted Linux server into a calmly operated local AI appliance.
> An operator describes the server once, picks a serving recipe, points it at a model, and gets a runnable profile that Unum validates against what the server can actually do.
> From then on, a gorgeous SSH TUI is the primary control surface.

Unum is **not**:

- a generic container orchestrator;
- a model marketplace;
- a vLLM / llama.cpp configuration API;
- a cloud control plane;
- a workstation desktop or browser app.

## Four pillars

### 1. Site layout — the server as deployment

The server has **named storage roles**, not one state directory. Each role can live on whichever disk fits.

- `config` — TOML config (e.g. `/etc/unum`).
- `state` — Unum-owned registries, host keys, logs (e.g. `/var/lib/unum`).
- `models` — host root for model repositories and model files.
- `profiles` — runnable profile YAML.
- `cache` — serving-stack scratch (HF cache, compile cache).

`unumd init` and `[storage]` config let operators place each role independently. Compose `volumes` remain the runtime source of truth; site layout supplies defaults and authoring context. Additional storage roles (e.g. for user-authored recipes) may emerge from pillar 3 without changing this principle.

### 2. Server inventory + profile validation — the server as constraint

Unum knows what *this* server actually offers, and validates profiles against it before they run.

- An operator-declared **inventory** records memory ceiling, accelerator devices (absolute host paths), declared port ranges, and optional cache roots.
- No autodetection beyond simple reporting; the operator is the source of truth.
- Profile validation runs against the inventory: device paths must be registered, memory limits must fit the ceiling, ports must not collide with reserved ranges.
- Validation errors say what is missing and what command would fix it.

This generalizes today's `[profiles].max_memory` into a real capability model.

### 3. Profile authoring via serving recipes — the workload as recipe

A **serving recipe** captures a known serving stack so operators don't write whole profile YAML files to try a new model or a different tuning.

- Recipes generate ordinary Compose-compatible profile YAML — they are an authoring aid, not a new runtime format.
- Recipes ship built-in, **and operators can add their own**. Unum is not a closed catalog.
- Serving-stack flags (quantization, AWQ, dynamic online quant, dtype, sliding window, tool-call parsers) stay inside recipes, not promoted to Unum CLI flags. Unum's vocabulary stays at the recipe/variant level.
- Model inputs are typed (Hugging Face repository directories, single files) so recipes can validate what they accept.

Recipe schema, storage, CLI surface, and built-in catalog scope are deferred to a follow-up design. This vision only commits to recipes being a first-class, extensible concept.

### 4. SSH TUI — the experience as craft

After `unumd init`, the **TUI is the product**.

- Dashboard, profiles, instances, logs (live-follow), tokens, server inventory, and recipe browser are reachable, consistent, keyboard-driven.
- Every error tells the operator what to do next.
- Visual craft is a goal, not a side effect: deliberate layout, terminal-aware typography, color used sparingly and meaningfully, small moments of delight that do not fight clarity.
- The CLI stays complete for scripting and onboarding; day-to-day operation lives in the TUI.

## How the vision relates to releases

- **Core** (complete) proved the thin end-to-end loop: profile → container → inference → TUI control. Core is the proof baseline, not the experience.
- **`v0.1.0`** is the first tagged release. The four pillars guide its planning, but exact `v0.1.0` scope and exit criteria are decided per work item — not by requiring every pillar to be fully realized. In particular, the recipes pillar's concrete scope depends on the follow-up recipe design.
- Active release scope lives in GitHub issues, ADRs, and current plans, not in this document.
- Pre-`v1.0.0`, prefer replacement over compatibility fallbacks; the pillars are durable, but their concrete surfaces can evolve.

## What this vision commits future work to

- Profile YAML stays Compose-compatible; recipes generate it, do not replace it.
- Storage roles are independent and operator-placed.
- Recipes are extensible by operators, not a closed catalog.
- Validation is server-aware, not just shape-aware.
- The TUI is the primary UX target; the CLI is the scripting surface.
- Serving-stack flags stay inside recipes/profiles; Unum does not grow flags per vLLM option.

## What this vision defers

- Recipe schema, storage, CLI surface, and built-in catalog scope (follow-up design).
- Recipe sharing or registry.
- Inventory autodiscovery beyond simple reporting.
- Multi-server federation.
- Per-recipe TUI wizards.
- Public internet exposure, RBAC/OAuth/OIDC, desktop/browser UI, Kubernetes — unchanged non-goals.

## Related documents

- Architecture: [`decisions/0001-v0-architecture.md`](decisions/0001-v0-architecture.md)
- Profile format: [`decisions/0002-profile-format-and-endpoints.md`](decisions/0002-profile-format-and-endpoints.md)
- Historical release framing: [`unum-v0-product-brief.md`](unum-v0-product-brief.md)
