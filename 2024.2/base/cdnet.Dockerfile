ARG DOTNET_BASE_TAG="7.0-bullseye-slim"
FROM mcr.microsoft.com/dotnet/sdk:$DOTNET_BASE_TAG

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true" \
    PATH="/opt/qodana:${PATH}"

ENV DOTNET_ROOT="/usr/share/dotnet"

# Not using the URL https://dot.net/v1/dotnet-install.sh because of https://github.com/dotnet/install-scripts/issues/276
ARG DOTNET_INSTALL_SH_REVISION="40434288dc5bbda41eafcbcbbc5c0fbbe028fb30"
ARG DOTNET_CHANNEL_A="7.0"
ARG DOTNET_CHANNEL_B="6.0"
ARG DOTNET_CHANNEL_C="8.0"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt/qodana /data/project /data/cache /data/results && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        default-jre \
        git \
        git-lfs \
        gnupg2 \
        locales && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    curl -fsSL -o /tmp/dotnet-install.sh  \
         "https://raw.githubusercontent.com/dotnet/install-scripts/$DOTNET_INSTALL_SH_REVISION/src/dotnet-install.sh" && \
    echo "d9ede6126a6da49cd3509e5fc8236f79addf175696f29d01f38840fd84663514 /tmp/dotnet-install.sh" > /tmp/shasum && \
    if [ "${DOTNET_INSTALL_SH_REVISION}" != "master" ]; then sha256sum --check --status /tmp/shasum; fi && \
    chmod +x /tmp/dotnet-install.sh && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_A -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_B -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_C -i $DOTNET_ROOT && \
    chmod 777 -R $DOTNET_ROOT && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*'
