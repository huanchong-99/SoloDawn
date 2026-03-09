# syntax=docker/dockerfile:1.7

ARG GITCORTEX_BUILD_NETWORK_PROFILE=official
ARG PNPM_VERSION=10.13.1
ARG RUST_TOOLCHAIN=nightly-2025-12-04
ARG NODE_MAJOR=22

# ============================================================
# Stage 1: Frontend build
# ============================================================
FROM node:${NODE_MAJOR}-slim AS frontend-builder
ARG GITCORTEX_BUILD_NETWORK_PROFILE
ARG PNPM_VERSION

WORKDIR /build

# Enable corepack pnpm — zero network cost, no npm install -g
RUN corepack enable && corepack prepare "pnpm@${PNPM_VERSION}" --activate

# China mirror for npm/pnpm (only affects pnpm install, not corepack)
RUN set -eux; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        npm config --global set registry https://registry.npmmirror.com; \
        pnpm config set registry https://registry.npmmirror.com --global; \
    fi

# Layer 1: dependency manifest only → cached unless lockfile changes
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY frontend/package.json frontend/

RUN --mount=type=cache,id=gitcortex-pnpm-store,target=/pnpm/store,sharing=locked \
    pnpm config set store-dir /pnpm/store && \
    cd frontend && \
    pnpm install --frozen-lockfile --prefer-offline

# Layer 2: source code → only rebuilds when src changes
COPY frontend/ frontend/
COPY shared/ shared/

ENV BROWSERSLIST_IGNORE_OLD_DATA=1 \
    GITCORTEX_BUILD_SOURCEMAP=0

RUN cd frontend && pnpm run build:docker

# ============================================================
# Stage 2: Rust toolchain + system deps (heavily cached layer)
# ============================================================
FROM rust:slim-trixie AS rust-base
ARG GITCORTEX_BUILD_NETWORK_PROFILE
ARG RUST_TOOLCHAIN

# Layer 1: apt dependencies — changes almost never, cached for months
RUN set -eux; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/*.sources 2>/dev/null || \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list 2>/dev/null || true; \
    fi; \
    printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
        > /etc/apt/apt.conf.d/80gitcortex-retries; \
    apt-get update && apt-get install -y --no-install-recommends \
        git pkg-config libsqlite3-dev libgit2-dev zlib1g-dev \
        cmake make ninja-build clang libclang-dev perl nasm libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# Layer 2: Rust toolchain — changes only when RUST_TOOLCHAIN arg changes
RUN set -eux; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        export RUSTUP_DIST_SERVER=https://rsproxy.cn; \
        export RUSTUP_UPDATE_ROOT=https://rsproxy.cn/rustup; \
    fi; \
    rustup set profile minimal && \
    rustup toolchain install "${RUST_TOOLCHAIN}" && \
    rustup default "${RUST_TOOLCHAIN}"

ENV SQLX_OFFLINE=true \
    CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse \
    CARGO_NET_RETRY=10 \
    CARGO_HTTP_TIMEOUT=120 \
    CARGO_HTTP_MULTIPLEXING=false \
    CARGO_NET_GIT_FETCH_WITH_CLI=true

# China cargo mirror
RUN set -eux; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        mkdir -p /usr/local/cargo && \
        printf '[source.crates-io]\nreplace-with = "rsproxy-sparse"\n\n[source.rsproxy-sparse]\nregistry = "sparse+https://rsproxy.cn/crates.io-index/"\n\n[registries.rsproxy-sparse]\nindex = "sparse+https://rsproxy.cn/crates.io-index/"\n' \
            >> /usr/local/cargo/config.toml; \
    fi

# ============================================================
# Stage 3: Rust dependency pre-build (skeleton pattern)
# Only re-runs when Cargo.toml/Cargo.lock/member list change
# ============================================================
FROM rust-base AS rust-deps
WORKDIR /build

# Copy workspace root manifests
COPY Cargo.toml Cargo.lock ./
COPY .cargo/config.toml .cargo/config.toml

# Copy ONLY Cargo.toml from each crate (not source code!)
COPY crates/server/Cargo.toml crates/server/Cargo.toml
COPY crates/db/Cargo.toml crates/db/Cargo.toml
COPY crates/services/Cargo.toml crates/services/Cargo.toml
COPY crates/executors/Cargo.toml crates/executors/Cargo.toml
COPY crates/utils/Cargo.toml crates/utils/Cargo.toml
COPY crates/local-deployment/Cargo.toml crates/local-deployment/Cargo.toml
COPY crates/deployment/Cargo.toml crates/deployment/Cargo.toml
COPY crates/review/Cargo.toml crates/review/Cargo.toml
COPY crates/cc-switch/Cargo.toml crates/cc-switch/Cargo.toml

# Generate minimal stub source files so cargo can resolve the dep graph
# This layer is only invalidated by Cargo.toml changes, NOT source changes
RUN set -eux; \
    for dir in crates/*/; do \
        crate_name="$(basename "$dir")"; \
        manifest="$dir/Cargo.toml"; \
        if [ ! -f "$manifest" ]; then continue; fi; \
        mkdir -p "$dir/src"; \
        if grep -q '\[\[bin\]\]' "$manifest" || grep -q 'src/main.rs' "$manifest" || [ "$crate_name" = "server" ]; then \
            echo 'fn main() {}' > "$dir/src/main.rs"; \
        fi; \
        echo '// stub' > "$dir/src/lib.rs"; \
    done

# Pre-compile all dependencies (not our code — stubs are trivial)
RUN --mount=type=cache,id=gitcortex-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=gitcortex-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    --mount=type=cache,id=gitcortex-cargo-target,target=/build/target,sharing=locked \
    cargo build --profile docker -p server 2>/dev/null || true

# ============================================================
# Stage 4: Rust final build (only recompiles changed crates)
# ============================================================
FROM rust-base AS rust-builder
WORKDIR /build

# Re-copy workspace manifests (needed for cargo)
COPY Cargo.toml Cargo.lock ./
COPY .cargo/config.toml .cargo/config.toml

# Copy ALL real source code — overwrites any stubs
COPY crates/ crates/
COPY assets/ assets/
COPY shared/ shared/
COPY --from=frontend-builder /build/frontend/dist frontend/dist

RUN --mount=type=cache,id=gitcortex-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=gitcortex-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    --mount=type=cache,id=gitcortex-cargo-target,target=/build/target,sharing=locked \
    cargo build --profile docker --locked -p server \
    && cp /build/target/docker/server /tmp/gitcortex-server

# ============================================================
# Stage 5: Runtime (minimal footprint)
# ============================================================
FROM debian:trixie-slim AS runtime
ARG GITCORTEX_BUILD_NETWORK_PROFILE
ARG INSTALL_AI_CLIS=0
ARG NODE_MAJOR=22

# Layer 1: system packages + Node.js (for AI CLI npm installs at runtime)
RUN set -eux; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/*.sources 2>/dev/null || \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list 2>/dev/null || true; \
    fi; \
    printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
        > /etc/apt/apt.conf.d/80gitcortex-retries; \
    apt-get update && apt-get install -y --no-install-recommends \
        libsqlite3-0 libgit2-1.9 git curl ca-certificates bash gnupg xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Install Node.js from nodesource — lightweight, single-purpose
RUN set -eux; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        NODE_DIST_URL="https://npmmirror.com/mirrors/node"; \
    else \
        NODE_DIST_URL="https://nodejs.org/dist"; \
    fi; \
    ARCH="$(dpkg --print-architecture)"; \
    case "$ARCH" in \
        amd64) NODE_ARCH="x64" ;; \
        arm64) NODE_ARCH="arm64" ;; \
        *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
    esac; \
    NODE_VERSION="$(curl -fsSL "${NODE_DIST_URL}/latest-v${NODE_MAJOR}.x/" | grep -oP 'node-v\K[0-9]+\.[0-9]+\.[0-9]+' | head -1)"; \
    curl -fsSL "${NODE_DIST_URL}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
        | tar -xJ --strip-components=1 -C /usr/local; \
    node --version && npm --version

RUN set -eux; \
    npm config --global set fetch-retries 5; \
    npm config --global set fetch-retry-mintimeout 20000; \
    npm config --global set fetch-retry-maxtimeout 120000; \
    npm config --global set fetch-timeout 300000; \
    if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
        npm config --global set registry https://registry.npmmirror.com; \
    fi

# Optional: gh CLI for copilot extension
RUN set -eux; \
    if [ "$INSTALL_AI_CLIS" = "1" ]; then \
        mkdir -p /etc/apt/keyrings; \
        if curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
            | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
            && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
            && apt-get update \
            && apt-get install -y --no-install-recommends gh; then \
            echo "Installed gh CLI"; \
        else \
            echo "Skipping gh CLI installation due to network or repository error"; \
            rm -f /etc/apt/sources.list.d/github-cli.list /usr/share/keyrings/githubcli-archive-keyring.gpg; \
        fi; \
        rm -rf /var/lib/apt/lists/*; \
    fi

ENV NPM_CONFIG_PREFIX=/var/lib/gitcortex/.npm-global \
    GH_EXTENSIONS_DIR=/var/lib/gitcortex/gh-extensions \
    PATH=/var/lib/gitcortex/.npm-global/bin:$PATH
RUN mkdir -p /var/lib/gitcortex/.npm-global /var/lib/gitcortex/gh-extensions

COPY scripts/docker/install/ /opt/gitcortex/install/
RUN if [ "$INSTALL_AI_CLIS" = "1" ]; then \
        bash /opt/gitcortex/install/install-ai-clis.sh; \
    else \
        echo "Skipping AI CLI installation during image build (INSTALL_AI_CLIS=$INSTALL_AI_CLIS)"; \
    fi

COPY --from=rust-builder /tmp/gitcortex-server /usr/local/bin/gitcortex-server
COPY assets/scripts/ /opt/gitcortex/assets/scripts/

RUN groupadd -r gitcortex && useradd -r -g gitcortex -m gitcortex \
    && mkdir -p /var/lib/gitcortex/assets /var/lib/gitcortex \
    && chown -R gitcortex:gitcortex /var/lib/gitcortex
VOLUME ["/var/lib/gitcortex"]

ENV GITCORTEX_ASSET_DIR=/var/lib/gitcortex/assets \
    GITCORTEX_TEMP_DIR=/var/lib/gitcortex \
    HOST=0.0.0.0 \
    PORT=23456 \
    RUST_LOG=info

EXPOSE 23456

COPY scripts/docker/entrypoint.sh /opt/gitcortex/entrypoint.sh
RUN chmod +x /opt/gitcortex/entrypoint.sh

USER gitcortex

ENTRYPOINT ["/opt/gitcortex/entrypoint.sh"]
CMD ["gitcortex-server"]
