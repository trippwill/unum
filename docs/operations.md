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
the CI-equivalent gate, installs `unumd` and the systemd unit, runs idempotent
init, enables the unit, and restarts `unumd`.

Initialize root-owned config and state:

```bash
sudo unumd init --config /etc/unum/unumd.toml --state /var/lib/unum --server-name unum
```

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
Profile memory validation defaults to `32g`; raise `[profiles].max_memory` only
for hosts intended to run larger profiles.

Before v1, config shape may still change. If upgrading an older test install,
remove stale `[inference].active_profile` entries; starting a profile now makes
it the running inference target.

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
