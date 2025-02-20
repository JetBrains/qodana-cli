FROM registry.jetbrains.team/p/sa/containers/qodana:cdnet-base-251

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/qodana-cdnet /opt/qodana/qodana

RUN apt-get update && \
    apt-get install -y sudo && \
    useradd -m -u 1001 -U qodana && \
    passwd -d qodana && \
    echo 'qodana ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

ENV PATH="/opt/qodana:${PATH}"

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for .NET Community"
WORKDIR /data/project
ENTRYPOINT ["qodana", "scan"]
