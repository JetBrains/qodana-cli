FROM registry.jetbrains.team/p/sa/containers/qodana:python-base-latest

ARG TARGETPLATFORM
ARG DEVICEID
ENV DEVICEID=$DEVICEID
COPY $TARGETPLATFORM $QODANA_DIST
RUN chmod +x $QODANA_DIST/bin/*.sh $QODANA_DIST/bin/qodana && \
    if ! python3 -I -c "import zipfile" 2>/dev/null; then \
        apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends python3 && \
        qd_python="installed"; \
    fi && \
    python3 -I /usr/local/lib/qodana/strip-distrusted-roots.py \
      "$QODANA_DIST"/plugins/python-ce/helpers/pip-*.whl \
      "$QODANA_DIST"/plugins/python-ce/helpers/virtualenv*.pyz \
      && \
    if [ "${qd_python:-}" = "installed" ]; then apt-get purge -y --auto-remove python3; fi && \
    update-alternatives --install /usr/bin/java java $JAVA_HOME/bin/java 0 && \
    update-alternatives --install /usr/bin/javac javac $JAVA_HOME/bin/javac 0 && \
    update-alternatives --set java $JAVA_HOME/bin/java && \
    update-alternatives --set javac $JAVA_HOME/bin/javac && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for Python (https://jb.gg/qodana-python)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]
