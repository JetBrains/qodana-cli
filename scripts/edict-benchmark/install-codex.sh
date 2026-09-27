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

# TeamCity supplies the native distribution and, for a custom run, the assembled CLI.
shopt -s nullglob
archives=("$benchmark_checkout/native-artifacts/"*.tar.gz)
[[ ${#archives[@]} -eq 1 ]] || { echo 'Expected one native distribution from the artifact dependency' >&2; exit 1; }
archive=$(basename "${archives[0]}")
(cd "$benchmark_checkout/native-artifacts" && sha256sum -c "$archive.sha256")
mkdir -p "$benchmark_output/native-unpacked" "$benchmark_output/tooling/bin"
[[ ! -e "$QODANA_DIST" ]] || { echo "Use a fresh native distribution directory: $QODANA_DIST" >&2; exit 1; }
tar -xzf "$benchmark_checkout/native-artifacts/$archive" -C "$benchmark_output/native-unpacked"
product_info=$(find "$benchmark_output/native-unpacked" -name product-info.json -print -quit)
[[ -n "$product_info" ]]
mv "$(dirname "$product_info")" "$QODANA_DIST"
printf "##teamcity[setParameter name='env.QODANA_DIST' value='%s']\n" "$QODANA_DIST"
runner_revision=$(git -C "$benchmark_source" rev-parse HEAD)
if [[ -n "${BENCHMARK_CLI_PATH:-}" ]]; then
  : "${BENCHMARK_CLI_REVISION:?Set the source revision of the assembled CLI}"
  [[ "$BENCHMARK_CLI_REVISION" == "$runner_revision" ]] || {
    echo "Assembled CLI revision $BENCHMARK_CLI_REVISION does not match checkout $runner_revision" >&2
    exit 1
  }
  cli_artifact="$BENCHMARK_CLI_PATH"
  [[ "$cli_artifact" == /* ]] || cli_artifact="$benchmark_checkout/$cli_artifact"
  [[ -f "$cli_artifact" ]] || { echo "CLI artifact not found: $cli_artifact" >&2; exit 1; }
  install -m 755 "$cli_artifact" "$benchmark_output/tooling/bin/qodana"
  printf 'Using assembled CLI from build %s, revision %s\n' "${BENCHMARK_CLI_BUILD_ID:-unknown}" "$runner_revision"
else
  if ! command -v go > /dev/null; then
    go_version=$(awk '$1 == "go" {print $2}' "$benchmark_source/go.mod")
    go_archive="go$go_version.linux-arm64.tar.gz"
    go_checksum=$(curl -fsSL 'https://go.dev/dl/?mode=json&include=all' \
      | jq -er --arg archive "$go_archive" '.[] | .files[] | select(.filename == $archive) | .sha256')
    curl -fsSL "https://go.dev/dl/$go_archive" -o "$benchmark_output/tooling/$go_archive"
    (cd "$benchmark_output/tooling" && printf '%s  %s\n' "$go_checksum" "$go_archive" | sha256sum -c -)
    tar -xzf "$benchmark_output/tooling/$go_archive" -C "$benchmark_output/tooling"
    export PATH="$benchmark_output/tooling/go/bin:$PATH"
  fi
  (cd "$benchmark_source" && go generate ./internal/tooling/... && go build -o "$benchmark_output/tooling/bin/qodana" ./cli)
fi
qodana --version
printf '%s\n' "$runner_revision" > "$benchmark_output/runner-revision.txt"
sha256sum "$benchmark_output/tooling/bin/qodana" > "$benchmark_output/cli.sha256"
printf '%s\n' "${BENCHMARK_CLI_BUILD_ID:-built-from-checkout}" > "$benchmark_output/cli-build.txt"
git -C "$benchmark_project" rev-parse HEAD > "$benchmark_output/source-revision.txt"
