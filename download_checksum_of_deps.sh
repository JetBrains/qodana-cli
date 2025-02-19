#!/bin/bash
set -euo pipefail

# Loads checksum of config-loader-cli.jar from maven to /tooling/ dir
# Execute this script LOCALLY ONLY, on CI/CD checksum file must be present

# Variables
CONFIG_LOADER_CLI_VERSION="0.0.9"
URL="https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/config-loader-cli/$CONFIG_LOADER_CLI_VERSION/config-loader-cli-$CONFIG_LOADER_CLI_VERSION.jar.sha256"
DEST_DIR="tooling"
SHA256_FILE="$DEST_DIR/config-loader-cli-$CONFIG_LOADER_CLI_VERSION.jar.sha256"

# Ensure the tooling directory exists
mkdir -p "$DEST_DIR"

# Remove other versions checksum files
find "$DEST_DIR" -name "config-loader-cli-*.jar.sha256" -type f -exec rm -f {} \;

# Download the SHA256 file
echo "Downloading the SHA256 file..."
curl -o "$SHA256_FILE" -L "$URL"
if [ $? -ne 0 ]; then
  echo "Error: Failed to download the SHA256 file."
  exit 1
fi
echo "SHA256 file downloaded: $SHA256_FILE"