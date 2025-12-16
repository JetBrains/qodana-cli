ARG BASE_TAG="bookworm-slim"
FROM debian:$BASE_TAG
ARG DEBIAN_FRONTEND=noninteractive

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DIST="/opt/idea" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true"

ENV JAVA_HOME="$QODANA_DIST/jbr" \
    QODANA_CONF="$HOME/.config/idea" \
    PATH="$QODANA_DIST/bin:$PATH"

# See also:
# https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-environment-variables#dotnet_root-dotnet_rootx86-dotnet_root_x86-dotnet_root_x64
# https://learn.microsoft.com/en-gb/dotnet/core/install/linux-scripted-manual#example
ENV DOTNET_ROOT="/usr/share/dotnet"
ENV PATH="$PATH:$DOTNET_ROOT:$DOTNET_ROOT/tools"

# Not using the URL https://dot.net/v1/dotnet-install.sh because of https://github.com/dotnet/install-scripts/issues/276
ARG DOTNET_INSTALL_SH_REVISION="2e497bbe880cf47b209fe0d1f9c5e051916f830e"
ARG DOTNET_INSTALL_SH_SHA256="3f30fbfa69e182be7e60fd0cd9189c53cb61799b6077159fec74341112f1715e"
ARG DOTNET_CHANNELS="8.0 9.0 10.0"

RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    bash <<-"EOF"
    set -euxo pipefail

    rm -f /etc/apt/apt.conf.d/docker-clean
    mkdir -m 777 -p "${QODANA_DATA}" "${QODANA_CONF}"

    apt-get update
    apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        fontconfig \
        default-jre \
        git \
        git-lfs \
        gnupg2 \
        locales \
        procps \
        software-properties-common

    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen
    locale-gen

    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd
    chmod 666 /etc/passwd

    git config --global --add safe.directory '*'

    # Install .NET SDKs ------------------------------------------------------------------------------------------------
    # System dependencies (see https://learn.microsoft.com/en-gb/dotnet/core/install/linux-debian?tabs=dotnet10#dependencies)
    apt-get install -y --no-install-recommends \
        libc6 \
        libgcc-s1 \
        libgssapi-krb5-2 \
        libicu72 \
        libssl3 \
        libstdc++6 \
        zlib1g

    # Download and verify install script
    dotnet_install_sh_url="https://raw.githubusercontent.com/dotnet/install-scripts/$DOTNET_INSTALL_SH_REVISION/src/dotnet-install.sh"
    curl -L "$dotnet_install_sh_url" -o /tmp/dotnet-install.sh
    actual_sha256=$(sha256sum /tmp/dotnet-install.sh | cut -d ' ' -f1)
    if [ "$DOTNET_INSTALL_SH_SHA256" != "$actual_sha256" ]; then
        echo "SHA 256 did not match for $dotnet_install_sh_url"
        echo "  expected: $DOTNET_INSTALL_SH_SHA256"
        echo "    actual: $actual_sha256"
        exit 1
    fi

    # Install .NET SDKs
    chmod +x /tmp/dotnet-install.sh
    for channel in $DOTNET_CHANNELS; do
        /tmp/dotnet-install.sh --channel $channel --install-dir "$DOTNET_ROOT"
    done

    # Verify that requested SDKs are installed and available
    installed_sdks=$(dotnet --list-sdks)
    for channel in $DOTNET_CHANNELS; do
        if ! grep -Eq "^$channel" <<< "$installed_sdks"; then
            echo "Could not find requested channel $channel in the output of 'dotnet --list-sdks':"
            echo "$installed_sdks"
            exit 1
        fi
    done

    # ------------------------------------------------------------------------------------------------------------------
    chmod 777 -R "$DOTNET_ROOT" "$HOME"

    # Cleanup
    apt-get autoremove --purge -y
EOF