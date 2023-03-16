FROM node:16-bullseye-slim

ARG TARGETPLATFORM

ENV HOME="/root" LC_ALL="en_US.UTF-8" QODANA_DIST="/opt/idea" QODANA_DATA="/data"
ENV JAVA_HOME="$QODANA_DIST/jbr" QODANA_DOCKER="true" QODANA_CONF="$HOME/.config/idea"

ENV PATH="$QODANA_DIST/bin:$PATH"

RUN mkdir -m 777 -p /opt $QODANA_DATA $QODANA_CONF && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      fontconfig \
      git \
      gnupg2 \
      locales && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*' && \
    npm install -g eslint && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc" && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

ARG QD_BUILD="QDJS-231.8246.36"
ARG QD_RELEASE="231"
RUN set -ex && \
    case "$TARGETPLATFORM" in \
        'linux/amd64') \
            OS_ARCH_SUFFIX=''; \
            ;; \
        'linux/arm64') \
            OS_ARCH_SUFFIX='-aarch64'; \
            ;; \
        *) echo "Unsupported architecture $TARGETPLATFORM or you forgot to enable Docker BuildKit" >&2; exit 1 ;; \
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

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for JS (https://jb.gg/qodana-js)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]