FROM registry.jetbrains.team/p/sa/containers/qodana:poly-base-latest

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

# Built twice: once plain, once with PRIVILEGED=true for the "-privileged" tag (see BuildXStep in the
# TeamCity config). The default stays "false" so the plain build really is unprivileged.
ARG PRIVILEGED="false"
RUN <<EOF
set -euxo pipefail
if [ "$PRIVILEGED" != "true" ]; then
    echo "Skipping privileged commands because PRIVILEGED is not set to true."
    exit 0
fi
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y sudo
DEBIAN_FRONTEND=noninteractive pam-auth-update --force
useradd -m -u 1001 -U qodana
passwd -d qodana
echo 'qodana ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
chmod 777 /etc/passwd
rm -rf /var/cache/apt /var/lib/apt/ /tmp/*
EOF

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana Poly (https://jb.gg/qodana-poly)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]
