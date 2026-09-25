#!/usr/bin/env bash
set -euo pipefail

if [[ "${BENCHMARK_PREFLIGHT:-false}" == true ]]; then
  echo 'Preflight only: comparison is not requested.'
  exit 0
fi
if [[ ! -f benchmark-output/qodana.sarif.json ]]; then
  echo 'Generation did not produce analysis SARIF; no comparison can be performed.'
  exit 0
fi
: "${BENCHMARK_COMPARISON_REVISION:?}"
[[ "$BENCHMARK_COMPARISON_REVISION" =~ ^[0-9a-f]{40}$ ]]
checkout_dir="$PWD"

# Deliberately check out comparison sources only after model execution has finished.
git init comparison-source
git -C comparison-source remote add origin https://github.com/JetBrains/qodana-cli.git
GIT_TERMINAL_PROMPT=0 git -C comparison-source fetch --depth=1 --filter=blob:none origin "$BENCHMARK_COMPARISON_REVISION"
git -C comparison-source sparse-checkout set edict/kotlin
git -C comparison-source checkout --detach FETCH_HEAD
test "$(git -C comparison-source rev-parse HEAD)" = "$BENCHMARK_COMPARISON_REVISION"
git -C comparison-source rev-parse HEAD > benchmark-output/comparison-revision.txt

export JAVA_HOME="${JDK_21_0:?JDK 21 is required for the comparison task}"
export PATH="$JAVA_HOME/bin:$PATH"
cd comparison-source/edict/kotlin
./gradlew --no-daemon --console=plain :benchmark:compare \
  -PbenchmarkDir="$checkout_dir/project/benchmark" \
  -PgenerationDir="$checkout_dir/benchmark-output"
