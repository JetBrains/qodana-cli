#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
: "${LITELLM_API_KEY:?}"
npm install --no-audit --no-fund --cache "$benchmark_output/npm-cache" \
  --prefix "$benchmark_output/tooling" "@openai/codex@${BENCHMARK_CODEX_VERSION:-0.155.1}"
codex --version
cat > "$benchmark_codex_home/config.toml" <<CONFIG
approval_policy = "never"
default_permissions = "edict-benchmark"
model = "${BENCHMARK_MODEL:-gpt-5.6-sol}"
model_reasoning_effort = "high"
model_provider = "litellm"

[model_providers.litellm]
name = "LiteLLM"
base_url = "https://litellm.labs.jb.gg/openai"
env_key = "LITELLM_API_KEY"
wire_api = "responses"

[permissions.edict-benchmark]
extends = ":read-only"
[permissions.edict-benchmark.filesystem]
":tmpdir" = "write"
"$benchmark_project" = "read"
"$benchmark_state" = "read"
"$benchmark_project/benchmark" = "deny"
"$benchmark_state/gold.sarif.json" = "deny"
"$benchmark_output/trace" = "deny"
"$benchmark_codex_home/sessions" = "deny"
"$benchmark_codex_home/log" = "deny"
"$benchmark_output/log" = "deny"
[permissions.edict-benchmark.workspace_roots]
"$benchmark_scratch" = true
[permissions.edict-benchmark.filesystem.":workspace_roots"]
"." = "write"
[permissions.edict-benchmark.network]
enabled = true
mode = "full"

[features]
multi_agent = true
[agents]
max_depth = 5
max_concurrent_threads_per_session = 6
CONFIG
chmod 600 "$benchmark_codex_home/config.toml"
