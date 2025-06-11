#!/bin/bash
set -euo pipefail

# Check if a release name is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <release_number>"
  exit 1
fi

# Get the release name from the first argument
RELEASE_NAME=$1
# Transform the release name (e.g., 2024.3 -> 243)
TRANSFORMED_NAME=$(echo "$RELEASE_NAME" | awk -F. '{print substr($1,3) $2}')
# Copy the 'next' directory to the new release directory
cp -r next "$RELEASE_NAME"
# Replace all instances of '-latest' with the transformed release name
# Using 'find' and 'perl' for compatibility
find "$RELEASE_NAME" -type f -exec perl -pi -e "s/-latest/-$TRANSFORMED_NAME/g" {} +

# Update related workflows: do not forget to update ci.yml AFTER publishing PUBLIC dockerfiles
YML_FILE=".github/workflows/base.yml"
if [ ! -f "$YML_FILE" ]; then
  echo "Error: $YML_FILE does not exist."
  exit 1
fi

append_version_to_yaml() {
  local version="$1"
  local yaml_file="$2"
  if yq e ".jobs.base.strategy.matrix.version[]" "$yaml_file" | grep -qx "$version"; then
    echo "Version $version already exists in $yaml_file"
  else
    yq e ".jobs.base.strategy.matrix.version += [\"$version\"]" -i "$yaml_file"
    echo "Updated $yaml_file with new version $version"
  fi
}

append_version_to_yaml "$RELEASE_NAME" "$YML_FILE"

echo "Done!"
