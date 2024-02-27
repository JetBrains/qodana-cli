ARG DOTNET_BASE_TAG="7.0-bullseye-slim"
FROM mcr.microsoft.com/dotnet/sdk:$DOTNET_BASE_TAG

# renovate: datasource=repology depName=debian_11/ca-certificates versioning=loose
ENV CA_CERTIFICATES_VERSION="20210119"
# renovate: datasource=repology depName=debian_11/curl versioning=loose
ENV CURL_VERSION="7.74.0-1.3+deb11u11"
# renovate: datasource=repology depName=debian_11/git versioning=loose
ENV GIT_VERSION="1:2.30.2-1+deb11u2"
# renovate: datasource=repology depName=debian_11/git-lfs versioning=loose
ENV GIT_LFS_VERSION="2.13.2-1+b5"
# renovate: datasource=repology depName=debian_11/gnupg2 versioning=loose
ENV GNUPG2_VERSION="2.2.27-2+deb11u2"
# renovate: datasource=repology depName=debian_11/default-jre versioning=loose
ENV DEFAULT_JRE_VERSION="2:1.11-72"

ENV QODANA_DATA="/data" \
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
        ca-certificates=$CA_CERTIFICATES_VERSION \
        curl=$CURL_VERSION \
        default-jre=$DEFAULT_JRE_VERSION \
        git=$GIT_VERSION \
        git-lfs=$GIT_LFS_VERSION \
        gnupg2=$GNUPG2_VERSION && \
    apt-get autoremove -y && apt-get clean && \
    curl -fsSL -o /tmp/dotnet-install.sh  \
         "https://raw.githubusercontent.com/dotnet/install-scripts/$DOTNET_INSTALL_SH_REVISION/src/dotnet-install.sh" && \
    echo "d9ede6126a6da49cd3509e5fc8236f79addf175696f29d01f38840fd84663514 /tmp/dotnet-install.sh" > /tmp/shasum && \
    if [ "${DOTNET_INSTALL_SH_REVISION}" != "master" ]; then sha256sum --check --status /tmp/shasum; fi && \
    chmod +x /tmp/dotnet-install.sh && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_A -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_B -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_C -i $DOTNET_ROOT && \
    chmod 777 -R $DOTNET_ROOT
