# Decision 0002: profile format and endpoints

## Status

Accepted.

Supersedes the custom TOML profile model in the product brief and the `.toml` profile storage shape implied by Decision 0001.

## Context

Unum profiles must replace hand-written `podman run` and `docker run` scripts for local AI workloads. Real profiles need runtime details such as images, host networking, devices, mounts, environment variables, shared memory, memory limits, entrypoints, shell commands, and security options.

Profiles also need Unum-specific metadata: profile identity, endpoint purpose, health checks, model paths, and which endpoint a remote agent or user should call.

## Decision

Use a Compose-compatible profile file as the v0 profile format, with Unum metadata in an `x-unum` extension block.

Each v0 profile file describes exactly one service. Unum remains an active-profile switcher, not a general orchestrator.

The service section owns container runtime shape:

- image;
- container name;
- host networking;
- devices;
- volumes and mounts;
- environment;
- memory and shm settings;
- security options;
- entrypoint and command.

The `x-unum` section owns product metadata:

- profile ID and display name;
- endpoint definitions;
- health checks;
- model paths or labels;
- device requirements that Unum should validate before launch.

Example:

```yaml
services:
  qwen3-coder:
    image: docker.io/intel/llm-scaler-vllm:0.14.0-b8.3.1
    container_name: unum-qwen3-coder
    network_mode: host
    security_opt: ["label=disable"]
    devices:
      - /dev/dri/renderD128:/dev/dri/renderD128
    volumes:
      - /laser/ai/models/llm-scaler:/llm/models:ro
      - /laser/ai/cache/hf:/root/.cache/huggingface
    shm_size: 8g
    mem_limit: 32g
    memswap_limit: 32g
    oom_score_adj: 900
    environment:
      HF_HOME: /root/.cache/huggingface
      VLLM_WORKER_MULTIPROC_METHOD: spawn
    entrypoint: /bin/bash
    command:
      - -lc
      - |
        source /opt/intel/oneapi/setvars.sh --force >/dev/null 2>&1 || true
        vllm serve /llm/models/Qwen3-Coder-30B-A3B-Instruct \
          --served-model-name battery-qwen3-coder-1x \
          --host 0.0.0.0 \
          --port 18081

x-unum:
  id: qwen3-coder-1x
  name: Qwen3 Coder 1x
  endpoints:
    openai:
      url: http://unum.internal:18081/v1
      health: /health
  models:
    - /laser/ai/models/llm-scaler/Qwen3-Coder-30B-A3B-Instruct
  required_devices:
    - /dev/dri/by-path/pci-0000:12:00.0-render
```

## Endpoint model

Profiles may expose multiple endpoints. Endpoints are profile-owned and may use different ports.

Endpoint `url` values are client-facing URLs that Unum displays and can health-check. They are not container-internal URLs. Local-only endpoints may use loopback; endpoints intended for remote agents should use a reachable host name such as `unum.internal`.

Unum should not force all inference traffic through one upstream port. Different ports are useful for remote agents because a request to a coding model should fail if that model is not running, rather than silently hitting whatever profile happens to be active.

Unum may still provide stable discovery, display, auth, and optional proxying for known endpoint kinds. The v0 product promise is:

```text
Unum serves one active profile at a time.
That profile may expose one or more explicit endpoints.
```

Web UIs such as ComfyUI or OpenWebUI are separate profiles when they need their own process. A ComfyUI profile can expose a `webui` endpoint on its own port, but it should not be mixed into a vLLM profile unless the same service actually serves both endpoints.

## Rejected alternatives

- **Fully custom TOML profile format:** rejected because it makes Unum own a container schema that Compose already covers.
- **Podman kube YAML:** rejected for v0 because it is Kubernetes-shaped, less natural for Docker viability, and introduces concepts Unum does not need.
- **Multiple profile formats in v0:** rejected until Compose-compatible profiles prove insufficient.
- **One fixed inference upstream port:** rejected because remote agents and web UIs need explicit endpoints and should fail safely when the requested profile is not running.

## Consequences

- The current custom TOML profile loader becomes transitional and should be replaced or migrated.
- Docker backend support becomes part of viable v0 planning because Compose is shared vocabulary across Docker and Podman.
- Profile validation must reject files with zero services or more than one service, then check the accepted Compose subset plus `x-unum` metadata.
- Unum must preserve configurable hostnames, IPs, paths, ports, TLS paths, and device mappings.
- Running `unumd` itself in a container remains a separate deployment-model decision because it affects Podman/Docker socket access, host networking, paths, and device visibility.
