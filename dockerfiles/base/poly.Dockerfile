# syntax=docker/dockerfile:1
ARG BASE_TAG="trixie-debian13-dev"
FROM dhi.io/debian-base:$BASE_TAG

ARG TARGETPLATFORM

# See also:
# https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-environment-variables#dotnet_root-dotnet_rootx86-dotnet_root_x86-dotnet_root_x64
# https://learn.microsoft.com/en-gb/dotnet/core/install/linux-scripted-manual#example
ENV DOTNET_ROOT="/usr/share/dotnet"
ENV PATH="$PATH:$DOTNET_ROOT:$DOTNET_ROOT/tools"

# Not using the URL https://dot.net/v1/dotnet-install.sh because of https://github.com/dotnet/install-scripts/issues/276
ARG DOTNET_INSTALL_SH_REVISION="2e497bbe880cf47b209fe0d1f9c5e051916f830e"
ARG DOTNET_INSTALL_SH_SHA256="3f30fbfa69e182be7e60fd0cd9189c53cb61799b6077159fec74341112f1715e"
ARG DOTNET_CHANNELS="8.0 9.0 10.0"

ENV HOME="/root" LC_ALL="en_US.UTF-8" QODANA_DIST="/opt/idea" QODANA_DATA="/data"
ENV JAVA_HOME="$QODANA_DIST/jbr" QODANA_DOCKER="true" QODANA_CONF="$HOME/.config/idea"

ENV PATH="$QODANA_DIST/bin:$PATH"

RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked <<EOF
set -euxo pipefail
rm -f /etc/apt/apt.conf.d/docker-clean
mkdir -m 777 -p /opt "$QODANA_DATA" "$QODANA_CONF"
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    fontconfig \
    gawk \
    git \
    git-lfs \
    gnupg2 \
    locales \
    libicu76 \
    openssh-client \
    procps \
    jq
echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen
locale-gen
apt-get autoremove -y
apt-get clean
chmod 777 -R "$HOME"
echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd
chmod 666 /etc/passwd
git config --global --add safe.directory '*'

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

# Remove the distrusted emSign ECC Root CA - G3 and AAA Certificate Services roots from the NuGet trust lists
# bundled with the SDKs. Drop this once https://github.com/dotnet/sdk/issues/56914 is fixed.
distrusted_roots=" 86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b d7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4 "
set +x
for pem in "$DOTNET_ROOT"/sdk/*/trustedroots/*.pem; do
    if ! grep -q -- '-----BEGIN CERTIFICATE-----' "$pem"; then
        echo "No PEM certificates in $pem, the trust list format has changed"
        exit 1
    fi
    # One part per certificate; the 8.0 SDK ships these files with CRLF line endings
    parts=$(mktemp -d)
    csplit --quiet --elide-empty-files --prefix "$parts/" --suffix-format '%05d' "$pem" '/-----BEGIN CERTIFICATE-----/' '{*}'
    for part in "$parts"/*; do
        fingerprint=$(tr -d '\r' < "$part" | sed -n '/-----BEGIN/,/-----END/{//!p}' | base64 -d | sha256sum | cut -d ' ' -f1)
        if [[ "$distrusted_roots" == *" $fingerprint "* ]]; then
            echo "Removing $fingerprint from $pem"
            rm "$part"
        fi
    done
    cat "$parts"/* > "$pem"
    rm -r "$parts"
done
set -x

# ------------------------------------------------------------------------------------------------------------------
chmod 777 -R "$DOTNET_ROOT" "$HOME"

rm -rf /tmp/*
EOF
