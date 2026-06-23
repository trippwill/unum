# Operating unum v0

## Install

Build on the server:

```bash
mise install
mise run precommit
sudo install -Dm755 bin/unumd /usr/local/bin/unumd
sudo install -Dm644 packaging/systemd/unumd.service /etc/systemd/system/unumd.service
```

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
`command=`, or `restrict` because v0 does not enforce those restrictions.
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
starter profile to the configured profiles directory. v0 accepts only the
documented subset in the example profile; unsupported Compose keys fail
validation instead of being ignored.

Validation:

```bash
sudo unumd profiles list --config /etc/unum/unumd.toml
sudo unumd profiles validate --config /etc/unum/unumd.toml qwen3-small-cpu
```

The starter profile uses `ghcr.io/ggml-org/llama.cpp:server` and expects this
model file:

```text
/var/lib/unum/models/Qwen3-0.6B-Q4_K_M.gguf
```

Validation fails until the `x-unum.models` paths exist.

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
