FROM registry.jetbrains.team/p/sa/containers/qodana:debian-base-latest

ARG TARGETPLATFORM
ARG DEVICEID
ENV DEVICEID=$DEVICEID
COPY $TARGETPLATFORM $QODANA_DIST
RUN chmod +x $QODANA_DIST/bin/*.sh $QODANA_DIST/bin/qodana && \
    update-alternatives --install /usr/bin/java java $JAVA_HOME/bin/java 0 && \
    update-alternatives --install /usr/bin/javac javac $JAVA_HOME/bin/javac 0 && \
    update-alternatives --set java $JAVA_HOME/bin/java && \
    update-alternatives --set javac $JAVA_HOME/bin/javac && \
    chmod 777 /etc/passwd && \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y libicu67=67.1-7 && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for C/C++ (https://jb.gg/qodana-cpp)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]
