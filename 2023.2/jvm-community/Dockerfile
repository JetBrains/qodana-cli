ARG BASE_TAG="bullseye-slim"
FROM debian:$BASE_TAG

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DIST="/opt/idea" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true"
ENV JAVA_HOME="$QODANA_DIST/jbr" \
    QODANA_CONF="$HOME/.config/idea" \
    PATH="$QODANA_DIST/bin:$PATH"

# hadolint ignore=SC2174
RUN mkdir -m 777 -p /opt $QODANA_DATA $QODANA_CONF && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates=20210119 \
        curl=7.74.0-1.3+deb11u7 \
        fontconfig=2.13.1-4.2 \
        git=1:2.30.2-1+deb11u2 \
        git-lfs=2.13.2-1+b5 \
        gnupg2=2.2.27-2+deb11u2 \
        locales=2.31-13+deb11u6 \
        procps=2:3.3.17-5 && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*' && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

ENV GRADLE_USER_HOME="/data/cache/gradle"

ARG QD_BUILD="QDJVMC-2023.2"
ARG QD_RELEASE="2023.2"
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

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana Community for JVM (https://jb.gg/qodana-jvm-community)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]