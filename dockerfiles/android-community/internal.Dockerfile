FROM registry.jetbrains.team/p/sa/containers/qodana:jvm-community-base-latest

ARG TARGETPLATFORM
ARG DEVICEID
ENV DEVICEID=$DEVICEID
COPY $TARGETPLATFORM $QODANA_DIST
RUN chmod +x $QODANA_DIST/bin/*.sh $QODANA_DIST/bin/qodana && \
    update-alternatives --install /usr/bin/java java $JAVA_HOME/bin/java 0 && \
    update-alternatives --install /usr/bin/javac javac $JAVA_HOME/bin/javac 0 && \
    update-alternatives --set java $JAVA_HOME/bin/java && \
    update-alternatives --set javac $JAVA_HOME/bin/javac && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/*

ENV ANDROID_SDK_ROOT="/opt/android-sdk" ANDROID_USER_HOME="$QODANA_DATA/cache/android"
ENV ANDROID_HOME="$ANDROID_SDK_ROOT"
ENV ANDROID_SDK_TOOLS="$ANDROID_SDK_ROOT/cmdline-tools/tools/bin" QODANA_CORETTO_SDK_11="/root/.jdks/corretto-11" QODANA_CORETTO_SDK_17="/root/.jdks/corretto-17"
# IDE includes JDK17 by default since 2022, so we need additional JDK for the most projects
COPY --from=amazoncorretto:11.0.33 /usr/lib/jvm/java-11-amazon-corretto $QODANA_CORETTO_SDK_11
COPY --from=amazoncorretto:17.0.21 /usr/lib/jvm/java-17-amazon-corretto $QODANA_CORETTO_SDK_17
# Both JDKs link lib/security/cacerts to the Amazon Linux trust store, which is not part of the JDK directories
COPY --from=amazoncorretto:17.0.21 /etc/pki/ca-trust/extracted/java/cacerts /etc/pki/java/cacerts

ARG ANDROID_SDK_VERSION="9123335"
ARG ANDROID_SDK_SHA256="0bebf59339eaa534f4217f8aa0972d14dc49e7207be225511073c661ae01da0a"
ARG ANDROID_API_LEVEL="33"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=SC2174
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends unzip && \
    mkdir -m 777 -p $QODANA_DATA/cache $ANDROID_USER_HOME $ANDROID_SDK_ROOT $ANDROID_SDK_ROOT/cmdline-tools $ANDROID_SDK_ROOT/platforms $ANDROID_SDK_ROOT/ndk && \
    echo "${ANDROID_SDK_SHA256} /tmp/android.zip" > /tmp/shasum && \
    curl -fsSL -o /tmp/android.zip  \
      "https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip" && \
    sha256sum --check --status /tmp/shasum && \
    unzip -q /tmp/android.zip -d ${ANDROID_SDK_ROOT}/cmdline-tools && \
    mv ${ANDROID_SDK_ROOT}/cmdline-tools/cmdline-tools ${ANDROID_SDK_ROOT}/cmdline-tools/tools && \
    echo y | ${ANDROID_SDK_TOOLS}/sdkmanager "platforms;android-${ANDROID_API_LEVEL}" && \
    chmod 777 -R $ANDROID_SDK_ROOT $HOME/.jdks/ && \
    distrusted_roots='SHA256: (86:A1:EC:BA:08:9C:4A:8D:3B:BE:27:34:C6:12:BA:34:1D:81:3E:04:3C:F9:E8:A8:62:CD:5C:57:A3:6B:BE:6B|D7:A7:A0:FB:5D:7E:27:31:D7:71:E9:48:4E:BC:DE:F7:1D:5F:0C:3E:0A:29:48:78:2B:C8:3E:E0:EA:69:9E:F4)$' && \
    for jdk in "$QODANA_CORETTO_SDK_11" "$QODANA_CORETTO_SDK_17"; do \
      certs=$("$jdk/bin/keytool" -list -v -cacerts -storepass changeit) && grep -q 'Alias name' <<< "$certs" || { echo "$jdk has no readable cacerts: $certs" >&2; exit 1; }; \
      if grep -qE "$distrusted_roots" <<< "$certs"; then echo "$jdk trusts a distrusted root CA (QD-16607)" >&2; exit 1; fi; \
    done && \
    apt-get purge --auto-remove -y unzip && \
    rm -rf /tmp/*

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana Community for Android (https://jb.gg/qodana-android)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]