FROM registry.jetbrains.team/p/sa/containers/qodana:debian-base-242

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
ENV ANDROID_SDK_TOOLS="$ANDROID_SDK_ROOT/cmdline-tools/tools/bin" QODANA_CORETTO_SDK="$QODANA_DATA/.jdks/corretto-11"
# IDE includes JDK17 by default since 2022, so we need additional JDK for the most projects
COPY --from=amazoncorretto:11.0.24 /usr/lib/jvm/java-11-amazon-corretto $QODANA_CORETTO_SDK

ARG ANDROID_SDK_VERSION="9123335"
ARG ANDROID_SDK_SHA256="0bebf59339eaa534f4217f8aa0972d14dc49e7207be225511073c661ae01da0a"
ARG ANDROID_API_LEVEL="33"
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
# hadolint ignore=SC2174
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends unzip=6.0-26+deb11u1 && \
    mkdir -m 777 -p $QODANA_DATA/cache $ANDROID_USER_HOME $ANDROID_SDK_ROOT $ANDROID_SDK_ROOT/cmdline-tools $ANDROID_SDK_ROOT/platforms $ANDROID_SDK_ROOT/ndk && \
    echo "${ANDROID_SDK_SHA256} /tmp/android.zip" > /tmp/shasum && \
    curl -fsSL -o /tmp/android.zip  \
      "https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_SDK_VERSION}_latest.zip" && \
    sha256sum --check --status /tmp/shasum && \
    unzip -q /tmp/android.zip -d ${ANDROID_SDK_ROOT}/cmdline-tools && \
    mv ${ANDROID_SDK_ROOT}/cmdline-tools/cmdline-tools ${ANDROID_SDK_ROOT}/cmdline-tools/tools && \
    echo y | ${ANDROID_SDK_TOOLS}/sdkmanager "platforms;android-${ANDROID_API_LEVEL}" && \
    chmod 777 -R $ANDROID_SDK_ROOT && \
    apt-get purge --auto-remove -y unzip && \
    rm -rf /tmp/*

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana Community for Android (https://jb.gg/qodana-android)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]