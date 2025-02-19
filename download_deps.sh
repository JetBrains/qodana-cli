#!/bin/bash
set -euo pipefail

# Variables
CONFIG_LOADER_CLI_VERSION="0.0.9"
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

# Function to calculate SHA256
function calculate_sha256 {
  local file="$1"

  if command -v sha256sum > /dev/null; then
    # Linux or macOS with sha256sum
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum > /dev/null; then
    # macOS with shasum
    shasum -a 256 "$file" | awk '{print $1}'
  elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
    # Windows with CertUtil
    certutil -hashfile "$file" SHA256 | findstr /v "SHA256" | tr -d '\r\n'
  else
    echo "Error: No supported hashing utility found!" >&2
    exit 1
  fi
}

# Compute and compare the SHA256 hashes
COMPUTED_HASH=$(calculate_sha256 "$JAR_FILE")
EXPECTED_HASH=$(cat "$HASH_FILE" | tr -d ' \r\n')

if [ "$COMPUTED_HASH" == "$EXPECTED_HASH" ]; then
  echo "SHA256 hash verification succeeded."
else
  echo "SHA256 hash verification failed."
  echo "Expected: $EXPECTED_HASH"
  echo "Got:      $COMPUTED_HASH"
  rm -f "$JAR_FILE"
  exit 1
fi