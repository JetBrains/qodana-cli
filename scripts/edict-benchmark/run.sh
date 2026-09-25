#!/usr/bin/env bash
set -euo pipefail

# Runs in the unmodified assembled image. Python orchestration stays on the CI agent.
if [[ $# -ne 7 ]]; then
  echo 'Usage: run.sh project output jar inspection-url model minutes preflight' >&2
  exit 2
fi
project_dir="$1"
output_dir="$2"
edict_jar="$3"
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
mkdir -p "$output_dir/classes"
# The assembled image contains Codex 0.117, predating the Kotlin host's permission profiles.
npm install --no-audit --no-fund --cache "$output_dir/npm-cache" --prefix "$output_dir/tooling" "@openai/codex@${BENCHMARK_CODEX_VERSION:-0.155.1}"
export PATH="$output_dir/tooling/node_modules/.bin:$PATH"
codex --version
javac -cp "$edict_jar" -d "$output_dir/classes" "$script_dir/BenchmarkHost.java"
exec java -cp "$output_dir/classes:$edict_jar" BenchmarkHost "$project_dir" "$output_dir" "$4" "$5" "$6" "$7"
