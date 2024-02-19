ARG NODE_TAG="16-bullseye-slim"
ARG PHP_TAG="8.2-cli-bullseye"
ARG COMPOSER_TAG="2.7.1"
FROM node:$NODE_TAG AS node_base
FROM composer:$COMPOSER_TAG AS composer_base
FROM php:$PHP_TAG

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
        ca-certificates=20210119 \
        curl=7.74.0-1.3+deb11u11 \
        fontconfig=2.13.1-4.2 \
        git=1:2.30.2-1+deb11u2 \
        git-lfs=2.13.2-1+b5 \
        gnupg2=2.2.27-2+deb11u2 \
        locales=2.31-13+deb11u8 \
        procps=2:3.3.17-5 \
        software-properties-common=0.96.20.2-2.1 && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*'

ENV PATH="/opt/yarn/bin:$PATH"
COPY --from=node_base /usr/local/bin/node /usr/local/bin/
COPY --from=node_base /usr/local/include/node /usr/local/include/node
COPY --from=node_base /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=node_base /opt/yarn-* /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    node --version && \
    npm --version && \
    yarn --version && \
    npm install -g eslint@v8.47.0 pnpm@v8.7.1 && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc"

COPY --from=composer_base /usr/bin/composer /usr/bin/composer