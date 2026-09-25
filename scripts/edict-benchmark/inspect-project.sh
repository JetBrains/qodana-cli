#!/usr/bin/env bash
# Generic native candidate execution for managed skill reviews; all writes stay in scratch.
set -euo pipefail
[[ $# == 3 ]] || { echo 'Usage: inspect-project.sh candidate.kts inspection-id scratch-output' >&2; exit 2; }
candidate=$(realpath "$1")
inspection_id="$2"
[[ "$inspection_id" =~ ^[A-Za-z0-9_][A-Za-z0-9_-]*$ ]]
: "${QODANA_DIST:?}"
: "${TMPDIR:?}"
mkdir -p "$3"
destination=$(realpath "$3")
[[ "$destination/" == "$(realpath "$TMPDIR")/"* ]] || { echo 'Output must be inside scratch' >&2; exit 2; }
# Codex starts in the source project; callers may override it with an explicit project path.
project="${EDICT_PROJECT_DIR:-$PWD}"
test -f "$project/pom.xml"
exec 9> "$TMPDIR/native-project-scan.lock"
flock -w 1800 9
# Long IDE system paths redirect the lock socket to /tmp, outside this sandbox.
workspace="$TMPDIR/q"
export JAVA_TOOL_OPTIONS="${JAVA_TOOL_OPTIONS:-} -Djava.io.tmpdir=\"$TMPDIR\""
if [[ ! -d "$workspace/project" ]]; then
  mkdir -p "$workspace/project"
  tar -C "$project" --exclude=.git --exclude=.edict --exclude=.qodana --exclude=benchmark \
  --exclude=inspections --exclude=target --exclude=qodana.yaml --exclude=AGENTS.md -cf - . | tar -C "$workspace/project" -xf -
  git -C "$workspace/project" init --quiet
fi
# Reuse one imported project and IDE cache across sequential candidates.
rm -rf "$workspace/project/inspections"
mkdir -p "$workspace/project/inspections"
cp "$candidate" "$workspace/project/inspections/$inspection_id.inspection.kts"
jq -n --arg id "$inspection_id" '{version:"1.0", profile:{name:"empty"},include:[{name:$id}]}' > "$workspace/project/qodana.yaml"
QODANA_CONF="$workspace/cache/config" timeout --signal=TERM --kill-after=30s 30m qodana scan \
  --within-docker=false --project-dir "$workspace/project" --results-dir "$destination/results" \
  --cache-dir "$workspace/cache" --disable-sanity --run-promo=false --save-report=false \
  --property=idea.headless.enable.statistics=false > "$destination/analysis.log" 2>&1
jq -e --arg id "$inspection_id" '[.runs[].tool | .driver, .extensions[]? | .rules[]?.id] | index($id) != null' \
  "$destination/results/qodana.sarif.json" > /dev/null
sha256sum "$candidate"
printf '%s\n' "$destination/results/qodana.sarif.json"
