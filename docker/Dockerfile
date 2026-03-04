# syntax=docker/dockerfile:1.7

# ============================================================
# Stage 1: Frontend build
# ============================================================
FROM node:22-slim AS frontend-builder
WORKDIR /build
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY frontend/package.json frontend/
RUN --mount=type=cache,id=gitcortex-pnpm-store,target=/pnpm/store,sharing=locked \
    corepack enable \
    && pnpm config set store-dir /pnpm/store \
    && cd frontend \
    && pnpm install --frozen-lockfile
COPY frontend/ frontend/
COPY shared/ shared/
ENV BROWSERSLIST_IGNORE_OLD_DATA=1
RUN cd frontend && pnpm build

# ============================================================
# Stage 2: Rust build
# ============================================================
FROM rust:slim-trixie AS rust-builder
RUN apt-get update && apt-get install -y \
    pkg-config libsqlite3-dev libgit2-dev zlib1g-dev \
    cmake ninja-build clang libclang-dev perl nasm libssl-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
ARG RUST_TOOLCHAIN=nightly-2025-12-04
COPY Cargo.toml Cargo.lock ./
RUN rustup set profile minimal \
    && rustup toolchain install ${RUST_TOOLCHAIN} \
    && rustup default ${RUST_TOOLCHAIN}
COPY crates/ crates/
COPY assets/ assets/
COPY shared/ shared/
COPY --from=frontend-builder /build/frontend/dist frontend/dist
COPY crates/db/.sqlx crates/db/.sqlx
ENV SQLX_OFFLINE=true \
    RUSTFLAGS=-Awarnings
RUN --mount=type=cache,id=gitcortex-cargo-registry,target=/usr/local/cargo/registry,sharing=locked \
    --mount=type=cache,id=gitcortex-cargo-git,target=/usr/local/cargo/git,sharing=locked \
    --mount=type=cache,id=gitcortex-cargo-target,target=/build/target,sharing=locked \
    cargo build --release --locked -p server \
    && cp /build/target/release/server /tmp/gitcortex-server

# ============================================================
# Stage 3: Runtime
# ============================================================
FROM debian:trixie-slim AS runtime
ARG INSTALL_AI_CLIS=0

RUN apt-get update && apt-get install -y \
    libsqlite3-0 libgit2-1.9 git curl ca-certificates bash gnupg \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
    | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" \
    > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/* \
    && curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update && apt-get install -y gh && rm -rf /var/lib/apt/lists/*

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
COPY assets/sounds/ /opt/gitcortex/assets/sounds/

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
