ARG NODE_TAG="20-bullseye-slim"
ARG PHP_TAG="8.3-cli-bullseye"
ARG COMPOSER_TAG="2.7.9"
FROM node:$NODE_TAG AS node_base
FROM composer:$COMPOSER_TAG AS composer_base
FROM php:$PHP_TAG

# renovate: datasource=repology depName=debian_11/ca-certificates versioning=loose
ENV CA_CERTIFICATES_VERSION="20210119"
# renovate: datasource=repology depName=debian_11/curl versioning=loose
ENV CURL_VERSION="7.74.0-1.3+deb11u13"
# renovate: datasource=repology depName=debian_11/fontconfig versioning=loose
ENV FONTCONFIG_VERSION="2.13.1-4.2"
# renovate: datasource=repology depName=debian_11/git versioning=loose
ENV GIT_VERSION="1:2.30.2-1+deb11u3"
# renovate: datasource=repology depName=debian_11/git-lfs versioning=loose
ENV GIT_LFS_VERSION="2.13.2-1+b5"
# renovate: datasource=repology depName=debian_11/gnupg2 versioning=loose
ENV GNUPG2_VERSION="2.2.27-2+deb11u2"
# renovate: datasource=repology depName=debian_11/locales versioning=loose
ENV LOCALES_VERSION="2.31-13+deb11u10"
# renovate: datasource=repology depName=debian_11/procps versioning=loose
ENV PROCPS_VERSION="2:3.3.17-5"
# renovate: datasource=repology depName=debian_11/software-properties-common versioning=loose
ENV SOFTWARE_PROPERTIES_VERSION="0.96.20.2-2.1"
# renovate: datasource=repology depName=debian_11/zip versioning=loose
ENV ZIP_VERSION="3.0-12"
# renovate: datasource=repology depName=debian_11/unzip versioning=loose
ENV UNZIP_VERSION="6.0-26+deb11u1"
# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="9.10.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="9.11.0"

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
        ca-certificates=$CA_CERTIFICATES_VERSION \
        curl=$CURL_VERSION \
        fontconfig=$FONTCONFIG_VERSION \
        git=$GIT_VERSION \
        git-lfs=$GIT_LFS_VERSION \
        gnupg2=$GNUPG2_VERSION \
        locales=$LOCALES_VERSION \
        procps=$PROCPS_VERSION \
        software-properties-common=$SOFTWARE_PROPERTIES_VERSION \
        zip=$ZIP_VERSION \
        unzip=$UNZIP_VERSION && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*'

ENV PATH="/opt/yarn/bin:$PATH"
ENV SKIP_YARN_COREPACK_CHECK=0
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
    npm install -g eslint@$ESLINT_VERSION pnpm@$PNPM_VERSION && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc"

COPY --from=composer_base /usr/bin/composer /usr/bin/composer