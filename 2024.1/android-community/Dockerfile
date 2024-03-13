ARG BASE_TAG="bullseye-slim"
ARG NODE_TAG="20-bullseye-slim"
FROM node:$NODE_TAG AS node_base
FROM debian:$BASE_TAG

# renovate: datasource=repology depName=debian_11/ca-certificates versioning=loose
ENV CA_CERTIFICATES_VERSION="20210119"
# renovate: datasource=repology depName=debian_11/curl versioning=loose
ENV CURL_VERSION="7.74.0-1.3+deb11u11"
# renovate: datasource=repology depName=debian_11/fontconfig versioning=loose
ENV FONTCONFIG_VERSION="2.13.1-4.2"
# renovate: datasource=repology depName=debian_11/git versioning=loose
ENV GIT_VERSION="1:2.30.2-1+deb11u2"
# renovate: datasource=repology depName=debian_11/git-lfs versioning=loose
ENV GIT_LFS_VERSION="2.13.2-1+b5"
# renovate: datasource=repology depName=debian_11/gnupg2 versioning=loose
ENV GNUPG2_VERSION="2.2.27-2+deb11u2"
# renovate: datasource=repology depName=debian_11/locales versioning=loose
ENV LOCALES_VERSION="2.31-13+deb11u8"
# renovate: datasource=repology depName=debian_11/procps versioning=loose
ENV PROCPS_VERSION="2:3.3.17-5"

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DIST="/opt/idea" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true"
ENV JAVA_HOME="$QODANA_DIST/jbr" \
    QODANA_CONF="$HOME/.config/idea" \
    PATH="$QODANA_DIST/bin:$PATH"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt $QODANA_DATA $QODANA_CONF && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates=$CA_CERTIFICATES_VERSION \
        curl=$CURL_VERSION \
        fontconfig=$FONTCONFIG_VERSION \
        git=$GIT_VERSION \
        git-lfs=$GIT_LFS_VERSION \
        gnupg2=$GNUPG2_VERSION \
        locales=$LOCALES_VERSION \
        procps=$PROCPS_VERSION && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*' && \
    mkdir -p -m 777 "$HOME/.m2" "$HOME/.m2/repository"

ENV GRADLE_USER_HOME="/data/cache/gradle"

ARG QD_RELEASE="2024.1"
ARG QD_BUILD="QDANDC-$QD_RELEASE"
# hadolint ignore=DL3003,SC2043
RUN set -ex && \
    dpkgArch="$(dpkg --print-architecture)" && \
    case "$dpkgArch" in \
        "amd64") \
            OS_ARCH_SUFFIX=""; \
            ;; \
        "arm64") \
            OS_ARCH_SUFFIX="-aarch64"; \
            ;; \
        *) echo "Unsupported architecture $dpkgArch" >&2; exit 1 ;; \
    esac && \
    QD_NAME="qodana-$QD_BUILD$OS_ARCH_SUFFIX" \
    QD_URL="https://download.jetbrains.com/qodana/$QD_RELEASE/$QD_NAME.tar.gz" && \
    curl -fsSL "$QD_URL" -o "/tmp/$QD_NAME.tar.gz" \
               "$QD_URL.sha256" -o "/tmp/$QD_NAME.tar.gz.sha256" \
               "$QD_URL.sha256.asc" -o "/tmp/$QD_NAME.tar.gz.sha256.asc" && \
    GNUPGHOME="$(mktemp -d)" && \
    export GNUPGHOME && \
    for key in \
        "B46DC71E03FEEB7F89D1F2491F7A8F87B9D8F501" \
      ; do \
        gpg --batch --keyserver "hkps://keys.openpgp.org" --recv-keys "$key" || \
        gpg --batch --keyserver "keyserver.ubuntu.com" --recv-keys "$key" ; \
    done && \
    gpg --verify "/tmp/$QD_NAME.tar.gz.sha256.asc" "/tmp/$QD_NAME.tar.gz.sha256" && \
    (cd /tmp && sha256sum --check --status "$QD_NAME.tar.gz.sha256") && \
    mkdir -p /tmp/qd && tar -xzf "/tmp/$QD_NAME.tar.gz" --directory /tmp/qd --strip-components=1 && \
    mv /tmp/qd/qodana-QD* "$QODANA_DIST" && \
    chmod +x "$QODANA_DIST"/bin/*.sh "$QODANA_DIST"/bin/qodana && \
    update-alternatives --install /usr/bin/java java "$JAVA_HOME/bin/java" 0 && \
    update-alternatives --install /usr/bin/javac javac "$JAVA_HOME/bin/javac" 0 && \
    update-alternatives --set java "$JAVA_HOME/bin/java" && \
    update-alternatives --set javac "$JAVA_HOME/bin/javac" && \
    apt-get purge --auto-remove -y gnupg2 && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/* "$GNUPGHOME"

ENV ANDROID_SDK_ROOT="/opt/android-sdk" ANDROID_USER_HOME="$QODANA_DATA/cache/android"
ENV ANDROID_HOME="$ANDROID_SDK_ROOT"
ENV ANDROID_SDK_TOOLS="$ANDROID_SDK_ROOT/cmdline-tools/tools/bin" QODANA_CORETTO_SDK="$QODANA_DATA/.jdks/corretto-11"
# IDE includes JDK17 by default since 2022, so we need additional JDK for the most projects
COPY --from=amazoncorretto:11.0.22 /usr/lib/jvm/java-11-amazon-corretto $QODANA_CORETTO_SDK

ARG ANDROID_SDK_VERSION="9123335"
ARG ANDROID_SDK_SHA256="0bebf59339eaa534f4217f8aa0972d14dc49e7207be225511073c661ae01da0a"
ARG ANDROID_API_LEVEL="33"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends unzip=6.0-26+deb11u1 && \
    mkdir -m 777 -p $QODANA_DATA/cache $ANDROID_USER_HOME $ANDROID_SDK_ROOT $ANDROID_SDK_ROOT/cmdline-tools $ANDROID_SDK_ROOT/platforms $ANDROID_SDK_ROOT/ndk && \
    echo "${ANDROID_SDK_SHA256} /tmp/android.zip" > /tmp/shasum && \
    curl -fsSL -o /tmp/android.zip  \
      "https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip" && \
    sha256sum --check --status /tmp/shasum && \
    unzip -q /tmp/android.zip -d ${ANDROID_SDK_ROOT}/cmdline-tools && \
    mv ${ANDROID_SDK_ROOT}/cmdline-tools/cmdline-tools ${ANDROID_SDK_ROOT}/cmdline-tools/tools && \
    echo y | ${ANDROID_SDK_TOOLS}/sdkmanager "platforms;android-${ANDROID_API_LEVEL}" && \
    chmod 777 -R $ANDROID_SDK_ROOT && \
    apt-get purge --auto-remove -y unzip && \
    rm -rf /tmp/*

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana Community for Android (https://jb.gg/qodana-android)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]