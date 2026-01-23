FROM registry.jetbrains.team/p/sa/containers/qodana:jvm-base-latest

ARG TARGETPLATFORM
ARG DEVICEID
ENV DEVICEID=$DEVICEID
ENV ANTHROPIC_BASE_URL=https://litellm.labs.jb.gg

COPY $TARGETPLATFORM $QODANA_DIST
RUN chmod +x $QODANA_DIST/bin/*.sh $QODANA_DIST/bin/qodana && \
    update-alternatives --install /usr/bin/java java $JAVA_HOME/bin/java 0 && \
    update-alternatives --install /usr/bin/javac javac $JAVA_HOME/bin/javac 0 && \
    update-alternatives --set java $JAVA_HOME/bin/java && \
    update-alternatives --set javac $JAVA_HOME/bin/javac && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

# Install Claude Code
RUN curl -fsSL https://claude.ai/install.sh | bash

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for JVM with Claude Code (https://jb.gg/qodana-jvm)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]
