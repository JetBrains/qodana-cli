#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

mkdir -p "$benchmark_output/tooling/bin"
(cd "$benchmark_source" && go build -o "$benchmark_output/tooling/bin/qodana" ./cli)

qodana --version
git -C "$benchmark_source" rev-parse HEAD > "$benchmark_output/runner-revision.txt"
sha256sum "$benchmark_output/tooling/bin/qodana" > "$benchmark_output/cli.sha256"
printf '%s\n' 'built-from-checkout' > "$benchmark_output/cli-build.txt"
git -C "$benchmark_project" rev-parse HEAD > "$benchmark_output/source-revision.txt"
