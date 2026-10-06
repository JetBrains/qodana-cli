#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
: "${LITELLM_API_KEY:?}"
npm install --no-audit --no-fund --cache "$benchmark_output/npm-cache" \
  --prefix "$benchmark_output/tooling" "@openai/codex@${BENCHMARK_CODEX_VERSION:-0.155.1}"
codex --version

# A temporary user-level Codex home: provider and project trust only. Edict settings come from
# `qodana edict install` into project/.codex, which Codex reads only for a trusted project.
cat > "$benchmark_codex_home/config.toml" <<CONFIG
model = "${BENCHMARK_MODEL:-gpt-5.6-sol}"
model_provider = "litellm"

[model_providers.litellm]
name = "LiteLLM"
base_url = "https://litellm.labs.jb.gg/openai"
env_key = "LITELLM_API_KEY"
wire_api = "responses"

[projects."$(cd "$benchmark_project" && pwd -P)"]
trust_level = "trusted"
CONFIG
chmod 600 "$benchmark_codex_home/config.toml"
