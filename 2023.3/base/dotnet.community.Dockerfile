ARG DOTNET_BASE_TAG="6.0-bullseye-slim"
FROM mcr.microsoft.com/dotnet/sdk:$DOTNET_BASE_TAG

ARG DOTNET_INSTALL_SH_REVISION="40434288dc5bbda41eafcbcbbc5c0fbbe028fb30"
ARG DOTNET_CHANNEL_A="7.0"
ARG DOTNET_CHANNEL_B="6.0"
ARG DOTNET_CHANNEL_C="8.0"

ENV PATH="/opt/qodana:${PATH}"
ENV DOTNET_ROOT="/usr/share/dotnet"
ENV QODANA_DOCKER true

RUN apt-get update && apt-get install -y git default-jre && \
    rm -rf /var/lib/apt/lists/* && mkdir -p /opt/qodana && \
    mkdir -p /data/project && mkdir -p /data/cache && mkdir -p /data/results && \
    curl -fsSL -o /tmp/dotnet-install.sh  \
         "https://raw.githubusercontent.com/dotnet/install-scripts/$DOTNET_INSTALL_SH_REVISION/src/dotnet-install.sh" && \
    echo "d9ede6126a6da49cd3509e5fc8236f79addf175696f29d01f38840fd84663514 /tmp/dotnet-install.sh" > /tmp/shasum && \
    if [ "${DOTNET_INSTALL_SH_REVISION}" != "master" ]; then sha256sum --check --status /tmp/shasum; fi && \
    chmod +x /tmp/dotnet-install.sh && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_A -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_B -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_C -i $DOTNET_ROOT && \
    chmod 777 -R $DOTNET_ROOT
