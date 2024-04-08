#
# Copyright 2021-2024 JetBrains s.r.o.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# This Dockerfile is for https://github.com/jetbrains/qodana-cli dev container.

FROM golang:bookworm
ARG TARGETARCH
ENV CHOCO_VERSION=2.2.2 \
    GH_VERSION=2.41.0
ENV MONO_REPO="https://download.mono-project.com/repo/debian" \
    MONO_KEY="3FA7E0328081BFF6A14DA29AA6A19B38D3D831EF"

RUN set -ex &&  \
    apt-get update && apt-get install -y --no-install-recommends \
      apt-transport-https \
      autoconf \
      automake \
      ca-certificates \
      cmake \
      curl \
      dpkg-dev \
      file \
      gnupg2 \
      iputils-ping \
      make \
      patch \
      libc6-dev \
      git \
      git-lfs \
      default-jre \
      xdg-utils

# note: choco is required for the full goreleaser functionality
RUN set -ex &&  \
    mkdir -p /tmp /opt/chocolatey &&  \
    gpg --homedir /tmp --no-default-keyring --keyring /usr/share/keyrings/mono-official-archive-keyring.gpg  \
        --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys $MONO_KEY &&  \
    echo "deb [trusted=yes] https://repo.goreleaser.com/apt/ /" | tee /etc/apt/sources.list.d/goreleaser.list &&  \
    echo "deb [signed-by=/usr/share/keyrings/mono-official-archive-keyring.gpg] $MONO_REPO stable-buster main" |  \
        tee /etc/apt/sources.list.d/mono-official-stable.list &&  \
    apt-get update && apt-get install --no-install-recommends goreleaser-pro mono-devel -y &&  \
    curl -sL "https://github.com/chocolatey/choco/releases/download/$CHOCO_VERSION/chocolatey.v$CHOCO_VERSION.tar.gz" |  \
        tar -xz -C "/opt/chocolatey" &&  \
    echo '#!/bin/bash' >> /usr/local/bin/choco &&  \
    echo 'mono /opt/chocolatey/choco.exe $@' >> /usr/local/bin/choco &&  \
    chmod +x /usr/local/bin/choco

# optional: the dependency for the internal needs
RUN curl -fsSL "https://codesign-distribution.labs.jb.gg/codesign-client-linux-$TARGETARCH"  \
        -o /usr/local/bin/codesign || true && \
    chmod +x /usr/local/bin/codesign || true

# optional: gh cli is a must only for publishing the releases
RUN curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_$TARGETARCH.deb"  \
        -o /tmp/gh.deb && \
    dpkg -i /tmp/gh.deb

# optional: docker is required for running all the tests
RUN curl -fsSL "https://get.docker.com" | sh

# optional: needed for .net tests
ARG DOTNET_INSTALL_SH_REVISION="40434288dc5bbda41eafcbcbbc5c0fbbe028fb30"
ENV DOTNET_ROOT="/usr/share/dotnet" \
    DOTNET_CHANNEL="6.0"
RUN curl -fsSL -o /tmp/dotnet-install.sh  \
             "https://raw.githubusercontent.com/dotnet/install-scripts/$DOTNET_INSTALL_SH_REVISION/src/dotnet-install.sh" && \
    echo "d9ede6126a6da49cd3509e5fc8236f79addf175696f29d01f38840fd84663514 /tmp/dotnet-install.sh" > /tmp/shasum && \
    if [ "${DOTNET_INSTALL_SH_REVISION}" != "master" ]; then sha256sum --check --status /tmp/shasum; fi && \
    chmod +x /tmp/dotnet-install.sh && \
    bash /tmp/dotnet-install.sh -c $DOTNET_CHANNEL -i $DOTNET_ROOT && \
    chmod 777 -R $DOTNET_ROOT

# optional: needed for cpp tests
ENV CXX="/usr/lib/llvm-16/bin/clang++" \
    CC="/usr/lib/llvm-16/bin/clang" \
    CPLUS_INCLUDE_PATH="/usr/lib/clang/16/include"
RUN echo "deb https://apt.llvm.org/bookworm llvm-toolchain-bookworm-16 main" \
        > /etc/apt/sources.list.d/llvm.list && \
    wget -qO /etc/apt/trusted.gpg.d/llvm.asc \
        https://apt.llvm.org/llvm-snapshot.gpg.key && \
    apt-get -qq update && \
    apt-get install -qqy -t \
      llvm-toolchain-bookworm-16 \
      clang-16 clang-tidy-16 \
      clang-format-16 lld-16 \
      libc++-16-dev \
      libc++abi-16-dev && \
    for f in /usr/lib/llvm-16/bin/*; do ln -sf "$f" /usr/bin; done && \
    ln -sf clang /usr/bin/cc && \
    ln -sf clang /usr/bin/c89 && \
    ln -sf clang /usr/bin/c99 && \
    ln -sf clang++ /usr/bin/c++ && \
    ln -sf clang++ /usr/bin/g++ && \
    rm -rf /var/lib/apt/lists/*

ENV PATH=$PATH:$DOTNET_ROOT:$DOTNET_ROOT/tools
RUN set -ex &&  \
    docker -h && choco -h && dotnet -h && goreleaser -h && gh --version &&  \
    git config --global --add safe.directory '*' && \
    apt-get purge --auto-remove -y gnupg && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/* "$GNUPGHOME"

LABEL maintainer="qodana-support@jetbrains.com" description="https://github.com/jetbrains/qodana-cli dev container"