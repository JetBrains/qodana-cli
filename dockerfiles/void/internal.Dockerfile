FROM registry.jetbrains.team/p/sa/containers/qodana:void-base-latest

ARG TARGETPLATFORM
ARG DEVICEID
ENV DEVICEID=$DEVICEID
COPY $TARGETPLATFORM $QODANA_DIST
RUN <<EOF
set -euxo pipefail
chmod +x "$QODANA_DIST"/bin/*.sh "$QODANA_DIST"/bin/qodana
update-alternatives --install /usr/bin/java java "$JAVA_HOME"/bin/java 0
update-alternatives --install /usr/bin/javac javac "$JAVA_HOME"/bin/javac 0
update-alternatives --set java "$JAVA_HOME"/bin/java
update-alternatives --set javac "$JAVA_HOME"/bin/javac
rm -rf /var/cache/apt /var/lib/apt/ /tmp/*
EOF

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for IJ Void (https://jb.gg/qodana-void)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]
