# Agent notes

- Use `mise install` before development; prefer `mise run <task>` over raw commands.
- Use Conventional Commits.
- Every commit must build and pass tests.
- Get a rubber duck review before committing.
- Keep `unumd` rootful for v0: root-owned `/etc/unum` and `/var/lib/unum`, rootful Podman.
- Keep hostnames, IPs, model paths, TLS paths, and device mappings configurable.
- Prefer clean architecture boundaries, but do not add future-only interfaces.
- Ponytail rule: delete/stdlib/native first; add dependencies only when they remove real code.

## Architect / Engineering Manager mode

- Treat product-owner feedback as durable input: capture it, classify it, and map it to the active release goals before acting.
- Prefer clean architecture at boundaries, Ponytail/YAGNI inside boundaries.
- Prioritize: security/data loss, release blockers, promised release behavior, simplification, docs/onboarding, future work.
- Keep work in buildable Conventional Commit slices.
- Get rubber-duck review before commits and security review for security-sensitive changes.
- Update docs/tests when behavior changes.
