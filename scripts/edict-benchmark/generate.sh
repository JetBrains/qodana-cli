#!/usr/bin/env bash
set -euo pipefail

: "${BENCHMARK_IMAGE:?}"
: "${BENCHMARK_CONTAINER:?}"
checkout_dir="$PWD"
output_dir="$checkout_dir/benchmark-output"
mkdir -p "$output_dir"
trap 'docker rm -f "$BENCHMARK_CONTAINER" >/dev/null 2>&1 || true' EXIT

# Both MCP servers and the Kotlin host run inside the unchanged assembled image.
docker run --rm --init --name "$BENCHMARK_CONTAINER" \
  --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  --user "$(id -u):$(id -g)" \
  -e QODANA_TOKEN -e LITELLM_API_KEY -e BENCHMARK_CODEX_VERSION \
  -e BENCHMARK_MODEL -e BENCHMARK_MINUTES -e BENCHMARK_LIMIT -e BENCHMARK_RULES -e BENCHMARK_PREFLIGHT \
  -e DEVICEID=200820300000000-0000-0000-0000-000000000001 \
  -v "$checkout_dir:$checkout_dir" -w "$checkout_dir" \
  --entrypoint /bin/bash "$BENCHMARK_IMAGE" "$checkout_dir/runner-source/scripts/edict-benchmark/run.sh"
