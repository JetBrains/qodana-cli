#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"
trap 'stop_servers' EXIT
trap 'exit 130' INT TERM
: "${QODANA_TOKEN:?}"
test -d "$benchmark_state/inbox"
mkdir -p "$benchmark_output/trace" "$benchmark_output/log/edict"
: >> "$benchmark_output/log/edict/edict-tasks.log"

qodana edict install --dest "$benchmark_codex_home/skills"
for skill_file in "$benchmark_codex_home"/skills/*/SKILL.md; do
  [[ -f "$skill_file" ]] || continue
  printf '\n[[skills.config]]\npath = "%s"\nenabled = true\n' "$skill_file" >> "$benchmark_codex_home/config.toml"
done

[[ -x "$benchmark_embedding_python" ]] || { echo "Embedding Python not found: $benchmark_embedding_python" >&2; exit 1; }
embedding_openmp=$(embedding_libgomp)
[[ -n "$embedding_openmp" ]] || { echo "Embedding environment has no libgomp runtime" >&2; exit 1; }

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
command = "$benchmark_output/tooling/bin/qodana"
args = ["edict", "mcp", "start", "--project-dir", "$benchmark_project", "--state-dir", "$benchmark_state", "--embedding-python", "$benchmark_embedding_python", "--log-dir", "$benchmark_output/log"]
env = { LD_PRELOAD = "$embedding_openmp${LD_PRELOAD:+:$LD_PRELOAD}" }
default_tools_approval_mode = "approve"
startup_timeout_sec = 90
required = true
[mcp_servers.inspection]
url = "$inspection_url"
default_tools_approval_mode = "approve"
tool_timeout_sec = 1800
required = true
CONFIG
echo "Inspection MCP is ready; managed Edict MCP will use existing state at $benchmark_state."
trap - EXIT INT TERM
