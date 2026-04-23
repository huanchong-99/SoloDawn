# syntax=docker/dockerfile:1.7

ARG SOLODAWN_BUILD_NETWORK_PROFILE=official
ARG PNPM_VERSION=10.13.1
ARG RUST_TOOLCHAIN=nightly-2025-12-04
ARG NODE_MAJOR=22

# ============================================================
# Stage 1: Rust toolchain + system deps (heavily cached layer)
# Moved before frontend-builder so that COPY --from=rust-base
# can be used to serialize BuildKit stages and prevent the
# daemon from crashing on resource-constrained machines.
# ============================================================
FROM rust:slim-trixie AS rust-base
ARG SOLODAWN_BUILD_NETWORK_PROFILE
ARG RUST_TOOLCHAIN

RUN set -eux; \
    if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/*.sources 2>/dev/null || \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list 2>/dev/null || true; \
    fi; \
    printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
        > /etc/apt/apt.conf.d/80solodawn-retries; \
    apt-get update && apt-get install -y --no-install-recommends \
        git pkg-config libsqlite3-dev libgit2-dev zlib1g-dev \
        cmake make ninja-build clang libclang-dev perl nasm libssl-dev \
        protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

RUN set -eux; \
    if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
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
    CARGO_NET_GIT_FETCH_WITH_CLI=true \
    AWS_LC_SYS_STATIC=1

RUN set -eux; \
    if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
        mkdir -p /usr/local/cargo && \
        printf '%s\n' \
            '[source.crates-io]' \
            'replace-with = "rsproxy-sparse"' \
            '' \
            '[source.rsproxy-sparse]' \
            'registry = "sparse+https://rsproxy.cn/crates.io-index/"' \
            '' \
            '[registries.rsproxy-sparse]' \
            'index = "sparse+https://rsproxy.cn/crates.io-index/"' \
            >> /usr/local/cargo/config.toml; \
    fi

# Install cargo-chef in rust-base (not a separate stage) to prevent
# BuildKit from running cargo-chef install in parallel with pnpm install.
RUN --mount=type=cache,id=solodawn-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=solodawn-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    cargo install cargo-chef --locked

# ============================================================
# Stage 2: Frontend build
# ============================================================
FROM node:${NODE_MAJOR}-slim AS frontend-builder
ARG SOLODAWN_BUILD_NETWORK_PROFILE
ARG PNPM_VERSION

WORKDIR /build

# Enable corepack pnpm — zero network cost, no npm install -g
RUN corepack enable && corepack prepare "pnpm@${PNPM_VERSION}" --activate

# China mirror for npm/pnpm (only affects pnpm install, not corepack)
RUN set -eux; \
    if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
        npm config --global set registry https://registry.npmmirror.com; \
        pnpm config set registry https://registry.npmmirror.com --global; \
    fi

# Layer 1: dependency manifest only → cached unless lockfile changes
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY frontend/package.json frontend/

# Force BuildKit to finish rust-base before starting pnpm install.
# Without this, BuildKit runs rust-base apt-get and pnpm install in
# parallel, which crashes the daemon on resource-constrained machines.
COPY --from=rust-base /etc/os-release /tmp/.buildkit-sync
RUN rm -f /tmp/.buildkit-sync

RUN --mount=type=cache,id=solodawn-pnpm-store,target=/pnpm/store,sharing=locked \
    pnpm config set store-dir /pnpm/store && \
    cd frontend && \
    ( pnpm install --frozen-lockfile --prefer-offline 2>&1 || \
      ( echo "pnpm install failed, clearing store and retrying..." && \
        rm -rf /pnpm/store/* && \
        pnpm install --frozen-lockfile ) )

# Layer 2: source code → only rebuilds when src changes
COPY frontend/ frontend/
COPY shared/ shared/

ENV BROWSERSLIST_IGNORE_OLD_DATA=1 \
    SOLODAWN_BUILD_SOURCEMAP=0

RUN cd frontend && pnpm run build:docker

# ============================================================
# Stage 3: cargo-chef planner
# Produces recipe.json — a dependency-only build plan.
# This stage is lightweight and rebuilds whenever Cargo
# manifests or crate structure changes.
# ============================================================
FROM rust-base AS chef-planner

WORKDIR /build

# Copy only what cargo-chef needs to analyze the dependency graph
COPY Cargo.toml Cargo.lock ./
COPY crates/ crates/
COPY .cargo/config.toml .cargo/config.toml
COPY proto/ proto/

RUN cargo chef prepare --recipe-path recipe.json

# ============================================================
# Stage 3b: cargo-chef cook + final build
# First: cook dependencies from recipe.json (heavily cached).
# Then:  build project source (only recompiles on src changes).
# ============================================================
FROM rust-base AS rust-builder

WORKDIR /build

# .cargo/config.toml must be present before cook so that
# build settings (env vars, rustflags, source mirrors) apply
# to dependency compilation as well.
COPY .cargo/config.toml .cargo/config.toml

# Copy the recipe from planner stage
COPY --from=chef-planner /build/recipe.json recipe.json

# Cook dependencies only (project source is NOT present yet).
# Uses --profile docker (opt-level=1, codegen-units=256) for
# faster builds. cargo-chef supports --profile since v0.1.62.
# BuildKit cache mounts keep the cargo registry/git warm.
RUN --mount=type=cache,id=solodawn-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=solodawn-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    cargo chef cook --profile docker --recipe-path recipe.json -p server

# Now copy the full project source code.
# Because dependencies are already compiled, only project
# crates will be recompiled on source changes.
COPY Cargo.toml Cargo.lock ./
COPY proto/ proto/
COPY crates/ crates/
COPY assets/ assets/
COPY shared/ shared/
COPY --from=frontend-builder /build/frontend/dist frontend/dist

RUN --mount=type=cache,id=solodawn-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=solodawn-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    cargo build --profile docker --locked -p server \
    && cp /build/target/docker/server /tmp/solodawn-server

# ============================================================
# Stage 4: Runtime (minimal footprint)
# ============================================================
FROM debian:trixie-slim AS runtime
ARG SOLODAWN_BUILD_NETWORK_PROFILE
ARG INSTALL_AI_CLIS=0
ARG NODE_MAJOR=22

# Force BuildKit to complete ALL prior stages before starting runtime.
# rust-builder depends on frontend-builder (for frontend/dist) and
# rust-base (for toolchain), so this single dependency serializes the
# entire build chain: rust-base → frontend-builder → rust-builder → runtime.
# Without this, BuildKit runs multiple stages in parallel, which crashes
# the Docker daemon on Windows 10 + WSL2 due to I/O pressure.
COPY --from=rust-builder /tmp/solodawn-server /tmp/.buildkit-sync
RUN rm -f /tmp/.buildkit-sync

RUN set -eux; \
    if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/*.sources 2>/dev/null || \
        sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list 2>/dev/null || true; \
    fi; \
    printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
        > /etc/apt/apt.conf.d/80solodawn-retries; \
    apt-get update && apt-get install -y --no-install-recommends \
        libsqlite3-0 libgit2-1.9 git curl ca-certificates bash gnupg xz-utils \
    && rm -rf /var/lib/apt/lists/* \
    && if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
        NODE_DIST_URL="https://npmmirror.com/mirrors/node"; \
    else \
        NODE_DIST_URL="https://nodejs.org/dist"; \
    fi \
    && ARCH="$(dpkg --print-architecture)" \
    && case "$ARCH" in \
        amd64) NODE_ARCH="x64" ;; \
        arm64) NODE_ARCH="arm64" ;; \
        *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
    esac \
    && NODE_VERSION="$(curl -fsSL \
        "${NODE_DIST_URL}/latest-v${NODE_MAJOR}.x/" \
        | grep -oP 'node-v\K[0-9]+\.[0-9]+\.[0-9]+' \
        | head -1)" \
    && curl -fsSL "${NODE_DIST_URL}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
        | tar -xJ --strip-components=1 -C /usr/local \
    && node --version && npm --version \
    && npm config --global set fetch-retries 5 \
    && npm config --global set fetch-retry-mintimeout 20000 \
    && npm config --global set fetch-retry-maxtimeout 120000 \
    && npm config --global set fetch-timeout 300000 \
    && if [ "$SOLODAWN_BUILD_NETWORK_PROFILE" = "china" ]; then \
        npm config --global set registry https://registry.npmmirror.com; \
    fi \
    && if [ "$INSTALL_AI_CLIS" = "1" ]; then \
        mkdir -p /etc/apt/keyrings; \
        if curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
            | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
            && GHCLI_ARCH="$(dpkg --print-architecture)" \
            && GHCLI_KR="/usr/share/keyrings/githubcli-archive-keyring.gpg" \
            && echo "deb [arch=${GHCLI_ARCH} signed-by=${GHCLI_KR}] https://cli.github.com/packages stable main" \
                > /etc/apt/sources.list.d/github-cli.list \
            && apt-get update \
            && apt-get install -y --no-install-recommends gh; then \
            echo "Installed gh CLI"; \
        else \
            echo "Skipping gh CLI installation due to network or repository error"; \
            rm -f /etc/apt/sources.list.d/github-cli.list /usr/share/keyrings/githubcli-archive-keyring.gpg; \
        fi; \
        rm -rf /var/lib/apt/lists/*; \
    fi

ENV NPM_CONFIG_PREFIX=/var/lib/solodawn/.npm-global \
    GH_EXTENSIONS_DIR=/var/lib/solodawn/gh-extensions \
    PATH=/var/lib/solodawn/.npm-global/bin:$PATH

COPY scripts/docker/install/ /opt/solodawn/install/
RUN mkdir -p /var/lib/solodawn/.npm-global \
        /var/lib/solodawn/gh-extensions \
    && if [ "$INSTALL_AI_CLIS" = "1" ]; then \
        bash /opt/solodawn/install/install-ai-clis.sh; \
    else \
        echo "Skipping AI CLI installation during image build (INSTALL_AI_CLIS=$INSTALL_AI_CLIS)"; \
    fi

COPY --from=rust-builder /tmp/solodawn-server /usr/local/bin/solodawn-server
COPY assets/scripts/ /opt/solodawn/assets/scripts/

COPY scripts/docker/entrypoint.sh /opt/solodawn/entrypoint.sh

# Create user, prepare ownership, and set USER BEFORE declaring VOLUME so
# that the volume inherits the correct ownership (avoids root-owned mount).
RUN groupadd -r solodawn && useradd -r -g solodawn -m solodawn \
    && mkdir -p /var/lib/solodawn/assets /var/lib/solodawn \
    && chown -R solodawn:solodawn /var/lib/solodawn \
    && chmod +x /opt/solodawn/entrypoint.sh
USER solodawn
VOLUME ["/var/lib/solodawn"]

ENV SOLODAWN_ASSET_DIR=/var/lib/solodawn/assets \
    SOLODAWN_TEMP_DIR=/var/lib/solodawn \
    HOST=0.0.0.0 \
    PORT=23456 \
    RUST_LOG=info

EXPOSE 23456

ENTRYPOINT ["/opt/solodawn/entrypoint.sh"]
CMD ["solodawn-server"]
