#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
cd "$benchmark_project"

# Agents must not read the benchmark answers or Codex's own session records.
qodana edict install --deny benchmark --deny .edict/gold.sarif.json \
  --deny "$benchmark_output/trace" --deny "$benchmark_codex_home/sessions" --deny "$benchmark_codex_home/log"

# The server listens on edict.mcpPort (default 27182), keeps .edict as its state and logs to log/process-log/<run-id>.
# It starts IntelliJ from QODANA_DIST on the first inspection call and stops it on exit.
# Its console output stays beside the run folders, which agents cannot read either.
server_log=log/process-log/edict-server.log
mkdir -p log/process-log
QODANA_CONF="$benchmark_output/mcp-config" nohup setsid qodana edict mcp start \
  --ide-wait-timeout 20m \
  --ide-property=-Xmx8g --ide-property=java.awt.headless=true --ide-property=idea.is.internal=true \
  --ide-property=eap.login.enabled=false > "$server_log" 2>&1 < /dev/null &
echo $! > "$benchmark_output/edict.pid"
for ((attempt=0; attempt<90; attempt++)); do
  kill -0 "$(cat "$benchmark_output/edict.pid")" 2>/dev/null || { echo 'Edict MCP exited during startup' >&2; exit 1; }
  ! grep -q 'edict-mcp listening at ' "$server_log" || break
  sleep 1
done
grep 'edict-mcp listening at ' "$server_log" || { echo 'Edict MCP startup timed out' >&2; exit 1; }
# The generation step streams this run's task log.
sed -n 's/.*Process log: //p' "$server_log" > "$benchmark_output/edict-process-log"
echo "Edict MCP server is ready; using existing state at $benchmark_state."
trap - EXIT INT TERM
