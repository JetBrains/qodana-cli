#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

shopt -s nullglob
archives=("$benchmark_checkout/native-artifacts/"*.tar.gz)
[[ ${#archives[@]} -eq 1 ]] || { echo 'Expected one native distribution from the artifact dependency' >&2; exit 1; }
archive=$(basename "${archives[0]}")
(cd "$benchmark_checkout/native-artifacts" && sha256sum -c "$archive.sha256")

mkdir -p "$benchmark_output/native-unpacked"
[[ ! -e "$QODANA_DIST" ]] || { echo "Use a fresh native distribution directory: $QODANA_DIST" >&2; exit 1; }
tar -xzf "$benchmark_checkout/native-artifacts/$archive" -C "$benchmark_output/native-unpacked"
product_info=$(find "$benchmark_output/native-unpacked" -name product-info.json -print -quit)
[[ -n "$product_info" ]] || { echo 'Native distribution has no product-info.json' >&2; exit 1; }
mv "$(dirname "$product_info")" "$QODANA_DIST"
printf "##teamcity[setParameter name='env.QODANA_DIST' value='%s']\n" "$QODANA_DIST"
