#!/usr/bin/env bash
set -euo pipefail

: "${BENCHMARK_SOURCE_REVISION:?}"
[[ "$BENCHMARK_SOURCE_REVISION" =~ ^[0-9a-f]{40}$ ]]
mkdir -p benchmark-output
git init runner-source
git -C runner-source remote add origin https://github.com/JetBrains/qodana-cli.git
GIT_TERMINAL_PROMPT=0 git -C runner-source fetch --depth=1 --filter=blob:none origin "$BENCHMARK_SOURCE_REVISION"
git -C runner-source sparse-checkout set edict/kotlin scripts/edict-benchmark
git -C runner-source checkout --detach FETCH_HEAD
test "$(git -C runner-source rev-parse HEAD)" = "$BENCHMARK_SOURCE_REVISION"
bash runner-source/scripts/edict-benchmark/prepare.sh
