ARG NODE_TAG="22-debian13-dev@sha256:56b70805f961b8ac98d3635c7c16591446f515683754c357e94b874280a3e6d2"
FROM dhi.io/node:$NODE_TAG

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="10.14.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="12.20.0"

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DIST="/opt/idea" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true"
ENV JAVA_HOME="$QODANA_DIST/jbr" \
    QODANA_CONF="$HOME/.config/idea" \
    PATH="$QODANA_DIST/bin:$PATH"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt $QODANA_DATA $QODANA_CONF && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        fontconfig \
        gawk \
        git \
        git-lfs \
        gnupg2 \
        gzip \
        locales \
        openssh-client \
        procps \
        jq && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*' && \
    node --version && \
    npm --version && \
    yarn --version && \
    npm install -g eslint@$ESLINT_VERSION pnpm@$PNPM_VERSION && npm config set update-notifier false && \
    pnpm --version && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc"

ENV SKIP_YARN_COREPACK_CHECK=0
