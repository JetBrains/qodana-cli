#!/usr/bin/env bash
set -euo pipefail

: "${BENCHMARK_IMAGE:?}"
: "${BENCHMARK_CONTAINER:?}"
checkout_dir="$PWD"
output_dir="$checkout_dir/benchmark-output"
mkdir -p "$output_dir"
trap 'docker rm -f "$BENCHMARK_CONTAINER" >/dev/null 2>&1 || true' EXIT

# Identical absolute paths let the host MCP proxy and container skills share artifacts.
# Both MCP endpoints bind only to loopback on the Linux build agent.
docker run -d --rm --init --name "$BENCHMARK_CONTAINER" --network host \
  --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  --user "$(id -u):$(id -g)" \
  -e QODANA_TOKEN -e LITELLM_API_KEY -e BENCHMARK_CODEX_VERSION \
  -e DEVICEID=200820300000000-0000-0000-0000-000000000001 \
  -v "$checkout_dir:$checkout_dir" -w "$checkout_dir" \
  --entrypoint /bin/sleep "$BENCHMARK_IMAGE" infinity

# Python is already available on the agent; nothing is installed in the Qodana image.
python3 benchmark-scripts/benchmark.py \
  --project "$checkout_dir/project" --output "$output_dir" \
  --jar "$checkout_dir/edict-runtime/edict-cli.jar" \
  --model "${BENCHMARK_MODEL:-gpt-5.6-sol}" --minutes "${BENCHMARK_MINUTES:-240}" \
  --limit "${BENCHMARK_LIMIT:-0}" --rules "${BENCHMARK_RULES:-}" \
  --preflight "${BENCHMARK_PREFLIGHT:-false}"
