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
max_concurrent_threads_per_session = 50
CONFIG
chmod 600 "$benchmark_codex_home/config.toml"

# TeamCity supplies the native distribution; build the CLI from this checkout
# so its embedded Kotlin server and skills match the benchmark sources.
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
qodana --version
git -C "$benchmark_source" rev-parse HEAD > "$benchmark_output/runner-revision.txt"
git -C "$benchmark_project" rev-parse HEAD > "$benchmark_output/source-revision.txt"
