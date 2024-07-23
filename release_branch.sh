#!/bin/bash

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

echo "Done!"