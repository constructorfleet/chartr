# Running chartr in Docker

chartr ships a single-user, self-hosted Docker image. This document is the
deep dive; `docs/getting-started.md` links here from a short "run in a
container" bullet.

## What you get

An image at `ghcr.io/rengwu/chartr:latest` (or `:edge` from `main`, or a
semver-shaped tag from a release) with:

- The chartr binary and its embedded Svelte cockpit UI, ready to serve
  on `:8787`.
- **Five baked agent CLIs**: Claude Code, OpenAI Codex, Gemini CLI,
  OpenCode, and Aider. Run `chartr agents list-baked` inside the
  container to enumerate them.
- **Debian bookworm-slim** base with Node 22 LTS, Python 3, and git,
  which is the runtime shape every agent CLI needs today.
- A **PID-1 reaper** (`tini`) and a **drop-privileges entrypoint**
  (`gosu`) so the container's `chartr` user matches your host uid/gid.
- Volumes for `/data` (session and runtime state) and `/config`
  (operator config: agent library, sources, preferences).
- A `HEALTHCHECK` polling `/api/health`.
- Multi-arch: `linux/amd64` and `linux/arm64`. Signed with cosign,
  attested with an SBOM and provenance.

## Quick start (Compose)

Copy `packaging/docker/compose.yaml` to a directory of your choice, drop a
`.env` file beside it with your agent auth tokens, and:

```sh
docker compose up -d
open http://localhost:8787   # macOS; xdg-open on Linux
```

Example `.env`:

```
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
GEMINI_API_KEY=...
UID=1000
GID=1000
```

## Quick start (one-liner)

```sh
docker run --rm -d \
    --name chartr \
    -p 127.0.0.1:8787:8787 \
    -e PUID=$(id -u) -e PGID=$(id -g) \
    -e CHARTR_PUBLIC_URL=http://localhost:8787 \
    -v chartr-data:/data \
    -v chartr-config:/config \
    -v "$HOME/src:/workspace" \
    --env-file ./.env \
    ghcr.io/rengwu/chartr:latest
```

## Configuration reference

Every knob is an environment variable. Every one has a working default so
`docker run` with nothing set brings up a working cockpit on
`http://localhost:8787`.

| Variable                | Default                | Notes |
|-------------------------|------------------------|-------|
| `CHARTR_ADDR`           | `0.0.0.0:8787`         | Bind. The image publishes 8787; loopback outside via `-p 127.0.0.1:8787:8787`. |
| `CHARTR_DATA_DIR`       | `/data`                | Session/runtime root — a named volume. |
| `XDG_CONFIG_HOME`       | `/config`              | chartr's operator config — a named volume. |
| `CHARTR_PUBLIC_URL`     | *(unset)*              | Additional URL(s) the host gate and websocket origin allowlist admit. Comma-separated. See "Remote access" below. |
| `CHARTR_IN_CONTAINER`   | `1`                    | Silences the loopback-exposure warning; the container's non-loopback bind IS the intended configuration. |
| `CHARTR_NO_PATH_PROBE`  | `1`                    | Skips the login-shell PATH hydrate — there is no operator login shell inside the container. |
| `PATH`                  | `/opt/agents:/usr/local/bin:/usr/bin:/bin` | Operator-supplied agents at `/opt/agents` shadow the baked set. |
| `PUID` / `PGID`         | `1000` / `1000`        | Reconciled with the container's `chartr` user at entrypoint. |

## Agent authentication

Two patterns work; use both if you like.

**Bind-mount the host's agent state.** Claude Code, Gemini CLI, Codex, and
OpenCode all persist auth to a per-agent directory under the operator's
home. Mount those directories into `/home/chartr/`:

```yaml
volumes:
  - ${HOME}/.claude:/home/chartr/.claude
  - ${HOME}/.config/gemini:/home/chartr/.config/gemini
```

**Pass API keys via `env_file`.** Aider reads `OPENAI_API_KEY` /
`ANTHROPIC_API_KEY`; Codex reads `OPENAI_API_KEY`; Claude Code accepts
`ANTHROPIC_API_KEY` for CI-style non-interactive use.

## Adding your own agents

Anything under `/opt/agents` prepends chartr's PATH inside the container.
An operator-supplied `codex` at `/opt/agents/codex` will run instead of
the baked one:

```yaml
volumes:
  - ./agents:/opt/agents:ro
```

If you register it in chartr's agent library (`config → agents`), it
shows up in the picker exactly like the baked five.

## Remote access

The container binds `0.0.0.0:8787` and `-p 127.0.0.1:8787:8787` keeps it
loopback-only on the host. Anything more than a same-machine browser is
your reverse-proxy problem; chartr has **no authentication**, and the
port IS the access check.

Set `CHARTR_PUBLIC_URL` to the URL your operator will visit — this
extends the host gate and websocket origin allowlist to match. The 30s
websocket ping (ticket 03) keeps long-lived PTYs alive through idle
proxies without extra headers.

**Caddy** (`Caddyfile`):

```
chartr.example.com {
    reverse_proxy 127.0.0.1:8787
}
```

**Traefik** (labels on a compose service):

```yaml
labels:
  - "traefik.enable=true"
  - "traefik.http.routers.chartr.rule=Host(`chartr.example.com`)"
  - "traefik.http.routers.chartr.tls=true"
  - "traefik.http.services.chartr.loadbalancer.server.port=8787"
```

In both cases:

```
CHARTR_PUBLIC_URL=https://chartr.example.com
```

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| **403 Forbidden** on every request through the proxy | Host gate rejected the public hostname | Set `CHARTR_PUBLIC_URL=https://your-name`. |
| Files in `/workspace` owned by root | Compose has `user:` set — bypasses the entrypoint | Remove `user:`; use `PUID`/`PGID` instead. |
| `chartr: agent not found` when spawning | `/opt/agents` shadowed a baked slug with a broken symlink | `ls /opt/agents/` inside the container; remove or fix. |
| Container restart loop with `unhealthy` | The healthcheck cannot reach `:8787` | `docker logs chartr` — usually a bad `CHARTR_ADDR` override. |
| Terminals hang after ~1 minute idle | Reverse proxy is idle-timing the websocket | Nothing to do server-side — chartr sends 30s pings. Ensure the proxy passes them (Caddy/Traefik do by default). |
| Agent CLI prompts for login every start | Auth state directory isn't persisted | Bind-mount the host's agent directory (see "Agent authentication"). |
| Blank page over HTTPS | Mixed content — SPA loaded via HTTPS but websocket dialed as `ws://` | Ensure `CHARTR_PUBLIC_URL` uses `https://` — chartr's SPA builds the ws URL from that. |

## Verify the image

The image is signed keylessly with cosign; verification pins the identity
to this repo's workflow.

```sh
cosign verify ghcr.io/rengwu/chartr:latest \
    --certificate-identity-regexp='^https://github.com/constructorfleet/chartr/.github/workflows/docker-publish.yml' \
    --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

The `verify` job in `.github/workflows/docker-publish.yml` runs the same
command after publish. A failure there does **not** auto-delete the bad
publish — that's a manual cleanup. See the workflow file for details.
