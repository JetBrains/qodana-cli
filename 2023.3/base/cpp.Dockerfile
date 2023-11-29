ARG BASE_TAG="bookworm-slim"

FROM debian:$BASE_TAG

ENV QODANA_DATA="/data" \
    QODANA_DOCKER="true"
ENV PATH="/opt/qodana:${PATH}"

ENV CXX="/usr/lib/llvm-16/bin/clang++" \
    CC="/usr/lib/llvm-16/bin/clang" \
    CPLUS_INCLUDE_PATH="/usr/lib/clang/16/include"

RUN apt-get -qq update; \
    apt-get install -qqy --no-install-recommends \
      gnupg2 \
      wget \
      ca-certificates \
      apt-transport-https \
      autoconf \
      automake \
      cmake \
      dpkg-dev \
      file \
      make \
      patch \
      libc6-dev \
      git \
      default-jre

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
    rm -rf /var/lib/apt/lists/* && \
    mkdir -p /opt/qodana $QODANA_DATA/project $QODANA_DATA/cache $QODANA_DATA/results
