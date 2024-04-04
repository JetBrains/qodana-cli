FROM golang:bookworm
ARG TARGETARCH
ENV CHOCO_VERSION=2.2.2 \
    GH_VERSION=2.41.0
ENV CHOCO_URL="https://github.com/chocolatey/choco/releases/download/$CHOCO_VERSION/chocolatey.v$CHOCO_VERSION.tar.gz" \
    GH_URL="https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_$TARGETARCH.deb" \
    CS_URL="https://codesign-distribution.labs.jb.gg/codesign-client-linux-$TARGETARCH" \
    MONO_REPO="https://download.mono-project.com/repo/debian" \
    MONO_KEY="3FA7E0328081BFF6A14DA29AA6A19B38D3D831EF" \
    GR_REPO="https://repo.goreleaser.com/apt/"

RUN set -ex \
    && mkdir -p /opt/chocolatey /tmp \
    && apt-get update \
    && apt-get install --no-install-recommends ca-certificates iputils-ping gnupg curl git openjdk-17-jre -y \
    && gpg --homedir /tmp --no-default-keyring --keyring /usr/share/keyrings/mono-official-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys $MONO_KEY \
    && echo "deb [trusted=yes] $GR_REPO /" | tee /etc/apt/sources.list.d/goreleaser.list \
    && echo "deb [signed-by=/usr/share/keyrings/mono-official-archive-keyring.gpg] $MONO_REPO stable-buster main" | tee /etc/apt/sources.list.d/mono-official-stable.list \
    && apt-get update && apt-get install --no-install-recommends goreleaser mono-devel -y \
    && curl -sL $CHOCO_URL | tar -xz -C "/opt/chocolatey" \
    && echo '#!/bin/bash' >> /usr/local/bin/choco \
    && echo 'mono /opt/chocolatey/choco.exe $@' >> /usr/local/bin/choco \
    && curl -fsSL $CS_URL -o /usr/local/bin/codesign \
    && curl -fsSL $GH_URL -o /tmp/gh.deb && dpkg -i /tmp/gh.deb \
    && chmod +x /usr/local/bin/choco /usr/local/bin/codesign \
    && codesign --help \
    && choco -h \
    && goreleaser -h \
    && gh --version \
    && git config --global --add safe.directory '*' \
    && apt-get purge --auto-remove -y gnupg \
    && rm -rf /var/cache/apt /var/lib/apt/ /tmp/* "$GNUPGHOME"