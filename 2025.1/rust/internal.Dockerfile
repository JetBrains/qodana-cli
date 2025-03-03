FROM registry.jetbrains.team/p/sa/containers/qodana:rust-base-251

ARG TARGETPLATFORM
ARG DEVICEID
ENV DEVICEID=$DEVICEID
COPY $TARGETPLATFORM $QODANA_DIST
RUN chmod +x $QODANA_DIST/bin/*.sh $QODANA_DIST/bin/qodana && \
    update-alternatives --install /usr/bin/java java $JAVA_HOME/bin/java 0 && \
    update-alternatives --install /usr/bin/javac javac $JAVA_HOME/bin/javac 0 && \
    update-alternatives --set java $JAVA_HOME/bin/java && \
    update-alternatives --set javac $JAVA_HOME/bin/javac && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*  && \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends unzip=6.0-26+deb11u1 build-essential && \
    plugin=$(curl -fSsL https://plugins.jetbrains.com/plugins/nightly/22407 | \
        awk 'BEGIN { RS="<idea-plugin"; FS="<download-url>|</download-url>" } /<version>233\./ && !found { print $2; found=1; }') && \
    curl -fSsL -o /tmp/plugin.zip "$plugin" && unzip /tmp/plugin.zip && mv intellij-rust $QODANA_DIST/plugins/ && \
    apt-get purge --auto-remove -y unzip && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

ARG PRIVILEGED="true"
RUN if [ "$PRIVILEGED" = "true" ]; then \
        apt-get update && \
        apt-get install -y sudo && \
        useradd -m -u 1001 -U qodana && \
        passwd -d qodana && \
        echo 'qodana ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers && \
        rm -rf /var/cache/apt /var/lib/apt/ /tmp/*; \
    else \
        echo "Skipping privileged commands because PRIVILEGED is not set to true."; \
    fi

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for Rust (https://jb.gg/qodana-rust)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]
