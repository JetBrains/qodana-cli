#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
: "${LITELLM_API_KEY:?}"
npm install --no-audit --no-fund --cache "$benchmark_output/npm-cache" \
  --prefix "$benchmark_output/tooling" "@openai/codex@${BENCHMARK_CODEX_VERSION:-0.155.1}"
codex --version

# The agent's default Python can be too old for the pinned embedding stack (NumPy 2.2
# requires Python 3.10+). Provision the benchmark environment explicitly so the MCP
# server never resolves dependencies against whichever python3 happens to be first on PATH.
uv="$benchmark_output/tooling/bin/uv"
if [[ ! -x "$uv" ]]; then
  curl -fsSL https://astral.sh/uv/0.8.22/install.sh \
    | env UV_INSTALL_DIR="$benchmark_output/tooling/bin" UV_NO_MODIFY_PATH=1 sh
fi
export UV_PYTHON_INSTALL_DIR="$benchmark_output/tooling/python"
"$uv" python install 3.12
"$uv" venv --python 3.12 --managed-python "$benchmark_embedding_venv"
"$uv" pip install --python "$benchmark_embedding_python" \
  -r "$benchmark_source/edict/kotlin/src/main/resources/distribution/requirements.txt"
embedding_openmp=$(embedding_libgomp)
[[ -n "$embedding_openmp" ]] || { echo "Embedding environment has no libgomp runtime" >&2; exit 1; }
LD_PRELOAD="$embedding_openmp${LD_PRELOAD:+:$LD_PRELOAD}" "$benchmark_embedding_python" -c \
  'import numpy, sentence_transformers; print(f"embedding Python ready: numpy={numpy.__version__}, sentence-transformers={sentence_transformers.__version__}")'

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
max_concurrent_threads_per_session = 50
CONFIG
chmod 600 "$benchmark_codex_home/config.toml"
