ARG RUST_TAG="1.83-slim-bullseye"
FROM rust:$RUST_TAG

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="9.15.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="9.14.4"

ARG TARGETPLATFORM

ENV HOME="/root" LC_ALL="en_US.UTF-8" QODANA_DIST="/opt/idea" QODANA_DATA="/data"
ENV JAVA_HOME="$QODANA_DIST/jbr" QODANA_DOCKER="true" QODANA_CONF="$HOME/.config/idea"

ENV PATH="$QODANA_DIST/bin:$PATH"

ARG RUSTUP_URL_AMD="https://static.rust-lang.org/rustup/dist/x86_64-unknown-linux-gnu/rustup-init"
ARG RUSTUP_URL_ARM="https://static.rust-lang.org/rustup/dist/aarch64-unknown-linux-gnu/rustup-init"

RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt $QODANA_DATA $QODANA_CONF && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends  \
        ca-certificates \
        curl \
        fontconfig \
        git \
        git-lfs \
        gnupg2 \
        locales \
        pkg-config \
        procps && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*' && \
    case $TARGETPLATFORM in \
      linux/amd64) RUSTUP_URL=$RUSTUP_URL_AMD;; \
      linux/arm64) RUSTUP_URL=$RUSTUP_URL_ARM;; \
      *) echo "Unsupported architecture $TARGETPLATFORM or you forgot to enable Docker BuildKit" >&2; exit 1;; \
    esac && \
    curl -fsSL -o /tmp/rustup-init $RUSTUP_URL && \
    chmod +x /tmp/rustup-init && /tmp/rustup-init -y --no-modify-path && \
    /usr/local/cargo/bin/rustup component add rust-src && \
    chmod -R +w /usr/local/rustup && \
    rm -rf /tmp/*