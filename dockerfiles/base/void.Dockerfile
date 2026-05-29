# syntax=docker/dockerfile:1
ARG BASE_TAG="trixie-debian13-dev"
FROM dhi.io/debian-base:$BASE_TAG

ARG TARGETPLATFORM

ENV HOME="/root" LC_ALL="en_US.UTF-8" QODANA_DIST="/opt/idea" QODANA_DATA="/data"
ENV JAVA_HOME="$QODANA_DIST/jbr" QODANA_DOCKER="true" QODANA_CONF="$HOME/.config/idea"

ENV PATH="$QODANA_DIST/bin:$CARGO_HOME/bin:$PATH"

RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked <<EOF
set -euxo pipefail
rm -f /etc/apt/apt.conf.d/docker-clean
mkdir -m 777 -p /opt "$QODANA_DATA" "$QODANA_CONF"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    fontconfig \
    gawk \
    git \
    git-lfs \
    gnupg2 \
    locales \
    openssh-client \
    pkg-config \
    procps \
    jq
echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen
locale-gen
apt-get autoremove -y
apt-get clean
chmod 777 -R "$HOME"
echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd
chmod 666 /etc/passwd
git config --global --add safe.directory '*'
rm -rf /tmp/*
EOF
