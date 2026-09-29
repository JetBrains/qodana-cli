#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

qodana edict install --dest "$benchmark_codex_home/skills"

[[ -x "$benchmark_embedding_launcher" ]] || { echo "Embedding Python launcher not found: $benchmark_embedding_launcher" >&2; exit 1; }
XDG_CACHE_HOME="$benchmark_output/cache" nohup setsid qodana edict mcp start \
  --project-dir "$benchmark_project" --state-dir "$benchmark_state" \
  --source-repository "$benchmark_state" \
  --embedding-python "$benchmark_embedding_launcher" \
  --log-dir "$benchmark_output/log" --http-port 0 > "$benchmark_output/log/edict-server.log" 2>&1 < /dev/null &
echo $! > "$benchmark_output/edict.pid"
edict_url=
for ((attempt=0; attempt<90; attempt++)); do
  kill -0 "$(cat "$benchmark_output/edict.pid")" 2>/dev/null || { echo 'Edict MCP exited during startup' >&2; exit 1; }
  edict_url=$(sed -nE 's/.*edict-mcp listening at (http[^[:space:]]+).*/\1/p' "$benchmark_output/log/edict-server.log" | head -1)
  [[ -z "$edict_url" ]] || break
  sleep 1
done
[[ -n "$edict_url" ]] || { echo 'Edict MCP startup timed out' >&2; exit 1; }

# QODANA_DIST selects the native distribution unpacked by the previous step.
inspection_url=$(
  QODANA_CONF="$benchmark_output/mcp-config" qodana edict linter-mcp start \
    --project-dir "$benchmark_project" --wait-timeout 20m \
    --state-file "$benchmark_output/inspection-state.json" --log-file "$benchmark_output/log/inspection-server.log" \
    --property=-Xmx8g --property=java.awt.headless=true --property=idea.is.internal=true --property=eap.login.enabled=false \
    | jq -er '.url'
)
cat >> "$benchmark_codex_home/config.toml" <<CONFIG

[mcp_servers.edict-mcp]
url = "$edict_url"
default_tools_approval_mode = "approve"
required = true
[mcp_servers.inspection]
url = "$inspection_url"
default_tools_approval_mode = "approve"
tool_timeout_sec = 1800
required = true
CONFIG
echo "Both MCP servers are ready; using existing state at $benchmark_state."
trap - EXIT INT TERM
