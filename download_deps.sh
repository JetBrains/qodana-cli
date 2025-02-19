#!/bin/bash
set -euo pipefail

# Variables
CONFIG_LOADER_CLI_VERSION="0.0.4"
URL="https://packages.jetbrains.team/maven/p/ij/intellij-dependencies/org/jetbrains/qodana/config-loader-cli/$CONFIG_LOADER_CLI_VERSION/config-loader-cli-$CONFIG_LOADER_CLI_VERSION.jar"
DEST_DIR="tooling"
JAR_FILE="$DEST_DIR/config-loader-cli.jar"
HASH_FILE="$DEST_DIR/config-loader-cli-$CONFIG_LOADER_CLI_VERSION.jar.sha256"

# Ensure the tooling directory exists
mkdir -p "$DEST_DIR"

# Download the JAR file
echo "Downloading the JAR file..."
curl -o "$JAR_FILE" -L "$URL"
if [ $? -ne 0 ]; then
  echo "Error: Failed to download the JAR file."
  exit 1
fi
echo "Download completed."

# Verify the SHA256 checksum
echo "Verifying the SHA256 checksum..."
if [ ! -f "$HASH_FILE" ]; then
  echo "Error: SHA256 checksum file not found: $HASH_FILE"
  exit 1
fi

# Compute the SHA256 hash of the downloaded file
COMPUTED_HASH=$(shasum -a 256 "$JAR_FILE" | awk '{print $1}')
EXPECTED_HASH=$(cat "$HASH_FILE" | tr -d ' \n')

if [ "$COMPUTED_HASH" == "$EXPECTED_HASH" ]; then
  echo "SHA256 hash verification succeeded."
else
  echo "SHA256 hash verification failed."
  echo "Expected: $EXPECTED_HASH"
  echo "Got:      $COMPUTED_HASH"
  rm -f "$JAR_FILE"
  exit 1
fi