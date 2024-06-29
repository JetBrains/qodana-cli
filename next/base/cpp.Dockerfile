ARG BASE_TAG="bookworm-slim"
FROM debian:$BASE_TAG
ARG CLANG="16"
# renovate: datasource=repology depName=debian_12/ca-certificates versioning=loose
ENV CA_CERTIFICATES_VERSION="20230311"
# renovate: datasource=repology depName=debian_12/curl versioning=loose
ENV CURL_VERSION="7.88.1-10+deb12u6"
# renovate: datasource=repology depName=debian_12/git versioning=loose
ENV GIT_VERSION="1:2.39.2-1.1"
# renovate: datasource=repology depName=debian_12/git-lfs versioning=loose
ENV GIT_LFS_VERSION="3.3.0-1+b5"
# renovate: datasource=repology depName=debian_12/gnupg2 versioning=loose
ENV GNUPG2_VERSION="2.2.40-1.1"
# renovate: datasource=repology depName=debian_12/default-jre versioning=loose
ENV DEFAULT_JRE_VERSION="2:1.17-74"
# renovate: datasource=repology depName=debian_12/apt-transport-https versioning=loose
ENV APT_TRANSPORT_HTTPS_VERSION="2.6.1"
# renovate: datasource=repology depName=debian_12/autoconf versioning=loose
ENV AUTOCONF_VERSION="2.71-3"
# renovate: datasource=repology depName=debian_12/automake versioning=loose
ENV AUTOMAKE_VERSION="1:1.16.5-1.3"
# renovate: datasource=repology depName=debian_12/cmake versioning=loose
ENV CMAKE_VERSION="3.25.1-1"
# renovate: datasource=repology depName=debian_12/dpkg-dev versioning=loose
ENV DPKG_DEV_VERSION="1.21.22"
# renovate: datasource=repology depName=debian_12/file versioning=loose
ENV FILE_VERSION="1:5.44-3"
# renovate: datasource=repology depName=debian_12/make versioning=loose
ENV MAKE_VERSION="4.3-4.1"
# renovate: datasource=repology depName=debian_12/patch versioning=loose
ENV PATCH_VERSION="2.7.6-7"
# renovate: datasource=repology depName=debian_12/libc6-dev versioning=loose
ENV LIBC6_DEV_VERSION="2.36-9+deb12u7"

ENV QODANA_DATA="/data" \
    QODANA_DOCKER="true"
ENV PATH="/opt/qodana:${PATH}"

ENV CXX="/usr/lib/llvm-$CLANG/bin/clang++" \
    CC="/usr/lib/llvm-$CLANG/bin/clang" \
    CPLUS_INCLUDE_PATH="/usr/lib/clang/$CLANG/include"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt/qodana $QODANA_DATA/project $QODANA_DATA/cache $QODANA_DATA/results && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates=$CA_CERTIFICATES_VERSION \
        curl=$CURL_VERSION \
        default-jre=$DEFAULT_JRE_VERSION \
        git=$GIT_VERSION \
        git-lfs=$GIT_LFS_VERSION \
        gnupg2=$GNUPG2_VERSION \
        apt-transport-https=$APT_TRANSPORT_HTTPS_VERSION \
        autoconf=$AUTOCONF_VERSION \
        automake=$AUTOMAKE_VERSION \
        cmake=$CMAKE_VERSION \
        dpkg-dev=$DPKG_DEV_VERSION \
        file=$FILE_VERSION \
        make=$MAKE_VERSION \
        patch=$PATCH_VERSION \
        libc6-dev=$LIBC6_DEV_VERSION && \
    apt-get autoremove -y && apt-get clean

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
