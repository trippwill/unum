# Agent notes

- Use `mise install` before development; prefer `mise run <task>` over raw commands.
- Use Conventional Commits.
- Every commit must build and pass tests.
- Get a rubber duck review before committing.
- Keep `unumd` rootful for v0: root-owned `/etc/unum` and `/var/lib/unum`, rootful Podman.
- Keep hostnames, IPs, model paths, TLS paths, and device mappings configurable.
- Prefer clean architecture boundaries, but do not add future-only interfaces.
- Ponytail rule: delete/stdlib/native first; add dependencies only when they remove real code.
