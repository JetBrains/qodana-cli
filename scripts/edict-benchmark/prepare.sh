#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
trap 'stop_servers' EXIT
trap 'exit 130' INT TERM
: "${QODANA_TOKEN:?}"
# The distribution and matching CLI are TeamCity artifact dependencies.
archive=qodana-QDJVM-263.SNAPSHOT.150-aarch64.tar.gz
(cd "$benchmark_checkout/native-artifacts" && sha256sum -c "$archive.sha256")
mkdir -p "$benchmark_output/native-unpacked" "$QODANA_DIST" "$benchmark_output/tooling/bin"
tar -xzf "$benchmark_checkout/native-artifacts/$archive" -C "$benchmark_output/native-unpacked"
product_info=$(find "$benchmark_output/native-unpacked" -name product-info.json -print -quit)
[[ -n "$product_info" ]]
cp -a "$(dirname "$product_info")/." "$QODANA_DIST/"
cp "$benchmark_checkout/native-cli/qodana" "$benchmark_output/tooling/bin/qodana"
chmod +x "$benchmark_output/tooling/bin/qodana"
qodana --version
git -C "$benchmark_source" rev-parse HEAD > "$benchmark_output/runner-revision.txt"
# Build the standard Edict application, not the benchmark reporting subproject.
"$benchmark_source/edict/kotlin/gradlew" -p "$benchmark_source/edict/kotlin" --no-daemon --console=plain installDist
edict="$benchmark_source/edict/kotlin/build/install/edict/bin/edict"
"$edict" install-skills --directory "$benchmark_codex_home/skills"
bash "$benchmark_source/scripts/edict-benchmark/import-inbox.sh"
cat > "$benchmark_project/AGENTS.md" <<CONTEXT
Source project: $benchmark_project
Managed state: $benchmark_output/state (write through edict-mcp).
Private scratch: $benchmark_scratch
Skills directory: $benchmark_codex_home/skills
Native inspections MCP provides compilation and example execution.
For complete project findings, use:
  bash $benchmark_source/scripts/edict-benchmark/inspect-project.sh <candidate.kts> <inspection-id> <scratch-output-directory>
Use an inspection ID distinct from built-in inspections (for example, prefix it with Edict).
CONTEXT
mkdir -p "$benchmark_output/trace" "$benchmark_output/mcp-cache" "$benchmark_output/mcp-results"
nohup setsid "$edict" mcp --project-dir "$benchmark_project" --state-dir "$benchmark_output/state" \
  --log-dir "$benchmark_output/log" --http-port 0 > "$benchmark_output/log/edict-server.log" 2>&1 < /dev/null &
echo $! > "$benchmark_output/edict.pid"
nohup setsid env QODANA_CONF="$benchmark_output/mcp-cache/config" qodana scan \
  --within-docker=false --project-dir "$benchmark_project" --results-dir "$benchmark_output/mcp-results" \
  --cache-dir "$benchmark_output/mcp-cache" --script mcp-server --profile-name empty \
  --disable-sanity --run-promo=false --save-report=false --property=idea.headless.enable.statistics=false \
  > "$benchmark_output/log/inspection-server.log" 2>&1 < /dev/null &
echo $! > "$benchmark_output/inspection.pid"
for ((attempt=0; attempt<1200; attempt++)); do
  kill -0 "$(cat "$benchmark_output/edict.pid")"
  kill -0 "$(cat "$benchmark_output/inspection.pid")"
  edict_url=$(sed -nE 's/.*edict-mcp listening at (http[^[:space:]]+).*/\1/p' "$benchmark_output/log/edict-server.log" | head -1)
  inspection_url=$(sed -nE 's/.*Streamable HTTP endpoint: (http[^[:space:]]+).*/\1/p' "$benchmark_output/log/inspection-server.log" | head -1 | sed $'s/\033.*//')
  if [[ -n "$edict_url" && -n "$inspection_url" ]]; then break; fi
  if ((attempt % 30 == 0)); then echo "Waiting for native MCP servers ($attempt seconds)"; fi
  sleep 1
done
[[ -n "$edict_url" && -n "$inspection_url" ]]
cat >> "$benchmark_codex_home/config.toml" <<CONFIG

[mcp_servers.edict-mcp]
url = "$edict_url"
default_tools_approval_mode = "approve"
required = true
[mcp_servers.inspection]
url = "$inspection_url"
enabled_tools = ["generate_psi_tree", "generate_inspection_kts_api", "generate_inspection_kts_examples", "run_inspection_kts"]
default_tools_approval_mode = "approve"
tool_timeout_sec = 1800
required = true
CONFIG
# Exercise the actual sandbox without starting a model request.
if env CODEX_HOME="$benchmark_codex_home" codex sandbox -P edict-benchmark -C "$benchmark_project" -- \
  /bin/sh -c 'printf allowed > "$1" && printf forbidden > "$2"' edict-probe \
  "$benchmark_scratch/sandbox-write-probe" "$benchmark_output/state/unmanaged-write-probe" \
  > "$benchmark_output/trace/sandbox.stdout" 2> "$benchmark_output/trace/sandbox.stderr"; then
  echo 'Sandbox incorrectly allowed an unmanaged state write' >&2
  exit 1
fi
test -s "$benchmark_scratch/sandbox-write-probe"
test ! -e "$benchmark_output/state/unmanaged-write-probe"
rm "$benchmark_scratch/sandbox-write-probe"
touch "$benchmark_output/servers-ready"
echo "Both native MCP servers are ready."
trap - EXIT INT TERM
