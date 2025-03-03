ARG BASE_TAG="bookworm-slim"
FROM debian:$BASE_TAG
ARG CLANG="16"

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DIST="/opt/idea" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true"

ENV PATH="/opt/qodana:$QODANA_DIST/bin:${PATH}"

ENV CXX="/usr/lib/llvm-$CLANG/bin/clang++" \
    CC="/usr/lib/llvm-$CLANG/bin/clang" \
    CPLUS_INCLUDE_PATH="/usr/lib/clang/$CLANG/include"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt/qodana $QODANA_DATA/project $QODANA_DATA/cache $QODANA_DATA/results && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        default-jre \
        git \
        git-lfs \
        gnupg2 \
        apt-transport-https \
        autoconf \
        automake \
        cmake \
        dpkg-dev \
        file \
        make \
        patch \
        libc6-dev \
        locales && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*'

RUN echo "deb https://apt.llvm.org/bookworm/ llvm-toolchain-bookworm-${CLANG} main" > /etc/apt/sources.list.d/llvm.list && \
    curl -s https://apt.llvm.org/llvm-snapshot.gpg.key | gpg --dearmor > /etc/apt/trusted.gpg.d/llvm.gpg && \
    apt-key adv --keyserver keyserver.ubuntu.com --recv-keys "15CF4D18AF4F7421" && \
    apt-get -qq update && \
    apt-get install -qqy -t \
      llvm-toolchain-bookworm-$CLANG \
      clang-$CLANG \
      clang-tidy-$CLANG \
      clang-format-$CLANG \
      lld-$CLANG \
      libc++-$CLANG-dev \
      libc++abi-$CLANG-dev && \
    for f in /usr/lib/llvm-$CLANG/bin/*; do ln -sf "$f" /usr/bin; done && \
    ln -sf clang /usr/bin/cc && \
    ln -sf clang /usr/bin/c89 && \
    ln -sf clang /usr/bin/c99 && \
    ln -sf clang++ /usr/bin/c++ && \
    ln -sf clang++ /usr/bin/g++ && \
    rm -rf /var/lib/apt/lists/* && \
    apt-get autoremove -y && apt-get clean
