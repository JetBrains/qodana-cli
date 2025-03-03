ARG CLANG="16"
FROM registry.jetbrains.team/p/sa/containers/qodana:cpp-community-base-$CLANG-latest

ARG TARGETPLATFORM
COPY $TARGETPLATFORM/qodana-clang /opt/qodana/qodana

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

ENV PATH="/opt/qodana:${PATH}"

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for C/C++ (CMake) (https://jb.gg/qodana-clang)"
WORKDIR /data/project
ENTRYPOINT ["qodana", "scan"]