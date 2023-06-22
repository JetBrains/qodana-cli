ARG NODE_TAG="16-bullseye-slim"
ARG BASE_TAG="6.0-bullseye-slim"

FROM node:$NODE_TAG AS node_base
FROM mcr.microsoft.com/dotnet/sdk:$BASE_TAG

ARG TARGETPLATFORM

ENV HOME="/root" LC_ALL="en_US.UTF-8" QODANA_DIST="/opt/idea" QODANA_DATA="/data"
ENV JAVA_HOME="$QODANA_DIST/jbr" QODANA_DOCKER="true" QODANA_CONF="$HOME/.config/idea"
ENV RIDER_UNREAL_ROOT="/data/unrealEngine" DOTNET_ROOT="/usr/share/dotnet"

# Not using the URL https://dot.net/v1/dotnet-install.sh because of https://github.com/dotnet/install-scripts/issues/276
ARG DOTNET_INSTALL_SH_REVISION="40434288dc5bbda41eafcbcbbc5c0fbbe028fb30"
ARG DOTNET_CHANNEL_A="7.0"
ARG DOTNET_CHANNEL_B="3.0"

ENV PATH="$QODANA_DIST/bin:/opt/yarn/bin:$PATH"

RUN mkdir -m 777 -p $QODANA_DATA $QODANA_CONF $DOTNET_ROOT $RIDER_UNREAL_ROOT && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      fontconfig \
      git \
      gnupg2 \
      locales \
      procps \
      software-properties-common && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*' && \
    curl -fsSL -o /tmp/dotnet-install.sh  \
      "https://raw.githubusercontent.com/dotnet/install-scripts/$DOTNET_INSTALL_SH_REVISION/src/dotnet-install.sh" && \
    echo "d9ede6126a6da49cd3509e5fc8236f79addf175696f29d01f38840fd84663514 /tmp/dotnet-install.sh" > /tmp/shasum && \
    if [ "${DOTNET_INSTALL_SH_REVISION}" != "master" ]; then sha256sum --check --status /tmp/shasum; fi && \
    chmod +x /tmp/dotnet-install.sh && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_A -i $DOTNET_ROOT && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL_B -i $DOTNET_ROOT && \
    chmod 777 -R $DOTNET_ROOT && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

COPY --from=node_base /usr/local/bin/node /usr/local/bin/
COPY --from=node_base /usr/local/include/node /usr/local/include/node
COPY --from=node_base /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=node_base /opt/yarn-* /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    node --version && \
    npm --version && \
    yarn --version

RUN npm install -g eslint && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc"

ARG QD_BUILD="QDNET-232"
ARG QD_RELEASE="2023.2"
RUN set -ex && \
    dpkgArch="$(dpkg --print-architecture)" && \
    case "$dpkgArch" in \
        'amd64') \
            OS_ARCH_SUFFIX=''; \
            ;; \
        'arm64') \
            OS_ARCH_SUFFIX='-aarch64'; \
            ;; \
        *) echo "Unsupported architecture $dpkgArch" >&2; exit 1 ;; \
    esac && \
    QD_NAME="qodana-$QD_BUILD$OS_ARCH_SUFFIX" \
    QD_URL="https://download.jetbrains.com/qodana/$QD_RELEASE/$QD_NAME.tar.gz" && \
    curl -fsSL "$QD_URL" -o /tmp/$QD_NAME.tar.gz \
        "$QD_URL.sha256" -o /tmp/$QD_NAME.tar.gz.sha256 \
        "$QD_URL.sha256.asc" -o /tmp/$QD_NAME.tar.gz.sha256.asc && \
    export GNUPGHOME="$(mktemp -d)" && \
    for key in \
        "B46DC71E03FEEB7F89D1F2491F7A8F87B9D8F501" \
      ; do \
        gpg --batch --keyserver "hkps://keys.openpgp.org" --recv-keys "$key" || \
        gpg --batch --keyserver "keyserver.ubuntu.com" --recv-keys "$key" ; \
    done && \
    gpg --verify /tmp/$QD_NAME.tar.gz.sha256.asc /tmp/$QD_NAME.tar.gz.sha256 && \
    (cd /tmp && sha256sum --check --status $QD_NAME.tar.gz.sha256) && \
    tar -xzf /tmp/$QD_NAME.tar.gz --directory /tmp/ --strip-components=1 && \
    mv /tmp/$QD_NAME $QODANA_DIST && \
    chmod +x $QODANA_DIST/bin/*.sh $QODANA_DIST/bin/qodana && \
    update-alternatives --install /usr/bin/java java $JAVA_HOME/bin/java 0 && \
    update-alternatives --install /usr/bin/javac javac $JAVA_HOME/bin/javac 0 && \
    update-alternatives --set java $JAVA_HOME/bin/java && \
    update-alternatives --set javac $JAVA_HOME/bin/javac && \
    apt-get purge --auto-remove -y gnupg2 && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/* $GNUPGHOME

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for .NET (https://jb.gg/qodana-dotnet)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]