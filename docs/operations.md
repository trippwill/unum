# Operating unum

## Install

Build on the server:

```bash
mise install
mise run precommit
sudo install -Dm755 bin/unumd /usr/local/bin/unumd
sudo install -Dm644 packaging/systemd/unumd.service /etc/systemd/system/unumd.service
```

For testing a pushed branch on a server that already has the repo checked out:

```bash
mise run dev-update -- feat/compose-profile-yaml
```

The helper refuses a dirty worktree, fetches and fast-forwards the branch, runs
the CI-equivalent gate, installs `unumd` and the systemd unit, runs `unumd init`
only when the config file is absent, enables the unit, and restarts `unumd`. To
re-initialize an existing config, run `sudo unumd init --overwrite` manually.

Initialize root-owned config and state:

```bash
sudo unumd init --config /etc/unum/unumd.toml --state /var/lib/unum --server-name unum
```

`unumd init` refuses to overwrite an existing config file. Pass `--overwrite` to
replace it; existing SSH host keys and starter profile files are always
preserved.

For incremental edits to an existing config, use `unumd config update` instead:

```bash
sudo unumd config update --memory-max 64g --device /dev/dri/renderD129
```

To dump the effective config (defaults plus what is loaded from disk):

```bash
sudo unumd config get
```

`unumd config update` only changes the fields you pass; everything else is
preserved as loaded. Machine values are validated before the file is rewritten
(atomically via a same-directory temp file + rename). It does not create or move
filesystem directories, and does not auto-derive the SSH host key path from
`--state` — for full reinitialization use `unumd init --overwrite`. The
`--device` flag replaces the entire device list when used (there is no
add/remove semantic).

`unumd config update` cannot clear a field back to empty; to remove a ceiling
or device list, edit the TOML directly or re-run `unumd init --overwrite`.

### Storage layout

`[storage]` defines four independent paths. Defaults are flat under
`/var/lib/unum`; each role can be placed on its own disk via the matching init
flag.

| Role | Default | Flag | Purpose |
| --- | --- | --- | --- |
| `state` | `/var/lib/unum` | `--state` | Unum-owned bookkeeping (registries, host keys, tokens, logs) |
| `profiles` | `/var/lib/unum/profiles` | `--profiles` | Runnable profile YAML |
| `models` | `/var/lib/unum/models` | `--models` | Host root for model repositories and files |
| `cache` | `/var/lib/unum/cache` | `--cache` | Serving-stack scratch (Hugging Face cache, compile cache, etc.) |

Each flag overrides exactly its own role. For a server with model storage on a
fast SSD and profiles on a ZFS pool, place each role explicitly:

```bash
sudo unumd init \
  --config /etc/unum/unumd.toml \
  --state /var/lib/unum \
  --models /fast/ai/models \
  --profiles /tank/ai/unum/profiles \
  --cache /fast/ai/unum/cache \
  --server-name unum \
  --overwrite
```

The SSH host key lives under `state` at `${state}/ssh/host_ed25519` by
convention; it is not a configurable storage role.

The `cache` role is created on init but is not yet consumed by any built-in
serving stack. It will be wired in when serving recipes land.

Import the admin user's existing authorized SSH keys:

```bash
sudo unumd ssh add-authorized-keys --config /etc/unum/unumd.toml --name admin /home/YOU/.ssh/authorized_keys
```

This registers each non-comment key as `admin-1`, `admin-2`, etc., and skips
keys already registered. To register one key only, use `ssh add-key` with a
single-key `.pub` file.

Unum rejects `authorized_keys` entries with OpenSSH options such as `from=`,
`command=`, or `restrict` because the current implementation does not enforce
those restrictions.
Unum also rejects SSH certificate entries for the same reason.

Start the daemon:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now unumd
```

Connect:

```bash
ssh -p 2222 unum.internal
```

## Config

Start from [`examples/unumd.toml`](../examples/unumd.toml). Hostnames, IPs,
ports, model paths, TLS paths, and device mappings are deployment config.

Before v1, config shape may still change. If upgrading an older test install,
remove stale `[inference].active_profile` entries; starting a profile now makes
it the running inference target.

### Machine capabilities

`[machine]` declares what the server actually offers. Profile validation
checks profiles against these declared capabilities before they start.

| Field | Default | Init flag | Purpose |
| --- | --- | --- | --- |
| `memory_max` | `32g` | `--memory-max` | Ceiling for `services.*.mem_limit` |
| `memswap_max` | `32g` | `--memswap-max` | Ceiling for `services.*.memswap_limit` |
| `cpus_max` | `"0"` | `--cpus-max` | Ceiling for `services.*.cpus` (fractional cores). `"0"` means unset; the check is skipped. |
| `devices` | empty | `--device PATH` (repeatable) | Registered absolute host device paths. Any profile referencing a device that is not in this list fails validation. |

The empty default for `devices` is strict on purpose: profiles that reference
devices must declare those devices in `[machine]` first. Edit
`/etc/unum/unumd.toml` or re-run `unumd init --overwrite --device /dev/dri/renderD129 ...`
to register hardware.

```bash
sudo unumd init \
  --config /etc/unum/unumd.toml \
  --memory-max 64g \
  --memswap-max 64g \
  --cpus-max 16 \
  --device /dev/dri/renderD129 \
  --device /dev/dri/by-path/pci-0000:12:00.0-render \
  --overwrite
```

For local smoke testing, loopback HTTP inference is allowed:

```toml
[inference]
address = "127.0.0.1:8770"
dev_insecure_http = true
```

For a non-loopback inference listener, set TLS paths and disable dev HTTP:

```toml
[inference]
address = "0.0.0.0:8770"
tls_cert = "/etc/unum/tls/server.crt"
tls_key = "/etc/unum/tls/server.key"
dev_insecure_http = false
```

## Profiles

Start from [`examples/profiles/qwen3-small-cpu.yaml`](../examples/profiles/qwen3-small-cpu.yaml).
Profiles are Compose-compatible YAML: put container runtime settings under
`services` and Unum metadata under `x-unum`. `unumd init` writes the same
starter profile to the configured profiles directory. The current implementation
accepts only the documented subset in the example profile;
unsupported Compose keys fail validation instead of being ignored.

Hardware-specific examples such as [`qwen3-coder-b60.yaml`](../examples/profiles/qwen3-coder-b60.yaml)
are examples only; copy and edit paths before installing them under the
configured profiles directory.

Validation:

```bash
sudo unumd profiles list --config /etc/unum/unumd.toml
sudo unumd profiles validate --config /etc/unum/unumd.toml qwen3-small-cpu
```

The starter profile uses `ghcr.io/ggml-org/llama.cpp:server` and bind-mounts
the init-created state model directory at `/models`. Put this model file there
for the smoke test:

```text
/var/lib/unum/models/Qwen_Qwen3-0.6B-Q4_K_M.gguf
```

Profile validation checks the Compose-shaped profile, not whether the command's
container-internal model path exists. Compose volumes and devices are the
runtime source of truth.

For a direct smoke-test download:

```bash
cd /var/lib/unum/models
curl -L \
  -o Qwen_Qwen3-0.6B-Q4_K_M.gguf \
  https://huggingface.co/bartowski/Qwen_Qwen3-0.6B-GGUF/resolve/main/Qwen_Qwen3-0.6B-Q4_K_M.gguf
```

Full Hugging Face repository downloads are also valid when profiles mount the
repository directory and point commands at the model file inside it.

Accelerators are explicit Compose profile config. Do not add the integrated
iGPU under `devices` unless you intend Unum workloads to use it.

## Inference tokens

Create and copy the token once:

```bash
sudo unumd tokens create --config /etc/unum/unumd.toml --name editor
```

Use it with OpenAI-compatible clients:

```bash
curl http://127.0.0.1:8770/openai/v1/chat/completions \
  -H "Authorization: Bearer unum_sk_..." \
  -H "Content-Type: application/json" \
  -d '{"model":"local","messages":[{"role":"user","content":"ping"}]}'
```

List or revoke:

```bash
sudo unumd tokens list --config /etc/unum/unumd.toml
sudo unumd tokens revoke --config /etc/unum/unumd.toml tok_...
```

## Smoke checks

```bash
sudo unumd serve --config /etc/unum/unumd.toml --check
sudo systemctl status unumd
ssh -p 2222 unum.internal
curl -i http://127.0.0.1:8770/openai/v1/models
```

The unauthenticated curl should return `401`. A valid token with no active
running profile should return `503`.

For a fast automated smoke test that exercises profile start, token validation,
and inference proxying without real Podman or model downloads:

```bash
mise run integration-smoke
```
