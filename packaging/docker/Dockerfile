# chartr — single-user, self-hosted Docker image.
#
# Three stages so the runtime image carries no toolchains:
#   1. web       — npm install + vite build → web/dist
#   2. builder   — go build with web/dist embedded → /out/chartr
#   3. runtime   — debian:bookworm-slim + node LTS + python3 + git,
#                  five baked agent CLIs, chartr binary, entrypoint
#
# See docs/docker.md for the operator surface (compose example, PUID/PGID,
# reverse-proxy snippets, cosign verify) and packaging/docker/compose.yaml
# for the canonical run shape.

# --- Stage 1: web -----------------------------------------------------------
FROM node:22-bookworm-slim AS web
WORKDIR /src

# Package files first so the layer cache survives a source-only edit.
COPY web/package.json web/package-lock.json* ./web/
RUN --mount=type=cache,target=/root/.npm \
    cd web && npm ci --no-audit --no-fund

COPY web ./web
RUN cd web && npm run build

# --- Stage 2: builder -------------------------------------------------------
FROM golang:1.26-bookworm AS builder
WORKDIR /src

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
# Overlay the built SPA on top of the checked-in web/dist/.gitkeep.
COPY --from=web /src/web/dist ./web/dist

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
      -o /out/chartr ./cmd/chartr

# --- Stage 3: runtime -------------------------------------------------------
FROM debian:bookworm-slim AS runtime

# NodeSource for a real node 22 LTS (Debian's is too old for several agent
# CLIs). Curl is only used at build time. tini and gosu land the PID-1 and
# drop-privileges patterns D4 spelled out; python3-pip installs aider.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ca-certificates curl git gnupg python3 python3-pip python3-venv \
        tini gosu \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && apt-get clean

# Baked agent CLIs — pinned by "@latest" at build time; the image digest is
# the reproducibility anchor (D9). Two package names remain flagged for
# implementation-time verification: @openai/codex and opencode-ai.
RUN --mount=type=cache,target=/root/.npm \
    npm install --global --no-audit --no-fund \
        @anthropic-ai/claude-code@latest \
        @openai/codex@latest \
        @google/gemini-cli@latest \
        opencode-ai@latest

# aider is python, not node. A dedicated venv keeps it off the system-python
# path so a `pip install` inside a running container never fights the OS
# packages.
RUN python3 -m venv /opt/aider && \
    /opt/aider/bin/pip install --no-cache-dir aider-chat && \
    ln -s /opt/aider/bin/aider /usr/local/bin/aider

# The manifest ticket 07 reads. One slug per line; ticket 07's list-baked
# subcommand skips blank lines and comments starting with #.
RUN mkdir -p /etc/chartr && \
    printf '%s\n' \
        '# chartr baked agents — see docs/docker.md' \
        'claude-code' \
        'codex' \
        'gemini-cli' \
        'opencode' \
        'aider' \
        > /etc/chartr/baked-agents

# The env contract for a container invocation. Every one of these is also
# overridable at run time — they are the defaults the image ships with.
ENV CHARTR_ADDR=0.0.0.0:8787 \
    CHARTR_DATA_DIR=/data \
    XDG_CONFIG_HOME=/config \
    CHARTR_NO_PATH_PROBE=1 \
    CHARTR_IN_CONTAINER=1 \
    PATH=/opt/agents:/usr/local/bin:/usr/bin:/bin \
    PUID=1000 \
    PGID=1000

# Named chartr user; the entrypoint reconciles its uid/gid with $PUID/$PGID.
RUN groupadd --gid 1000 chartr && \
    useradd --uid 1000 --gid chartr --home /home/chartr --create-home --shell /bin/sh chartr && \
    mkdir -p /data /config /opt/agents /workspace && \
    chown -R chartr:chartr /data /config /home/chartr

COPY --from=builder /out/chartr /usr/local/bin/chartr
COPY packaging/docker/chartr-entrypoint /usr/local/bin/chartr-entrypoint
RUN chmod +x /usr/local/bin/chartr /usr/local/bin/chartr-entrypoint

VOLUME ["/data", "/config"]
EXPOSE 8787

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8787/api/health || exit 1

# OCI labels — D6/D7 populate image.version and image.revision at build time
# via docker/metadata-action; chartr.agents mirrors the manifest above.
LABEL org.opencontainers.image.title="chartr" \
      org.opencontainers.image.description="Self-hosted single-user chartr cockpit" \
      org.opencontainers.image.source="https://github.com/constructorfleet/chartr" \
      org.opencontainers.image.licenses="MIT" \
      chartr.agents="claude-code,codex,gemini-cli,opencode,aider"

USER root
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/chartr-entrypoint"]
