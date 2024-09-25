ARG CLANG="16"
FROM registry.jetbrains.team/p/sa/containers/qodana:cpp-base-$CLANG-242

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/qodana-clang /opt/qodana/qodana

RUN apt-get update && \
    apt-get install -y sudo && \
    useradd -m -u 1001 -U qodana && \
    passwd -d qodana && \
    echo 'qodana ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers

ENV PATH="/opt/qodana:${PATH}"

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for C/C++ (CMake) (https://jb.gg/qodana-clang)"
WORKDIR /data/project
ENTRYPOINT ["qodana", "scan"]