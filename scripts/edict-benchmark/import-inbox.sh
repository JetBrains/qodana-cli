#!/usr/bin/env bash
# Import only required labelled examples. Optional examples and gold stay held out.
set -euo pipefail
source "$(dirname "$0")/common.sh"
[[ ! -d "$benchmark_output/state" ]] || { echo 'Use a fresh output directory' >&2; exit 1; }
revision=$(git -C "$benchmark_project" rev-parse HEAD)
jq -s --arg rules "${BENCHMARK_RULES:-}" --argjson limit "${BENCHMARK_LIMIT:-0}" '
  ($rules | split(",") | map(select(length > 0))) as $selected |
  if (($selected - map(.ruleId)) | length) > 0 then error("Unknown benchmark rule") else . end |
  map(select($selected == [] or (.ruleId as $id | $selected | index($id)))) |
  sort_by(.ruleId) | if $limit > 0 then .[:$limit] else . end |
  if length == 0 then error("No benchmark specifications") else . end |
  if all(.[]; .ruleId | test("^[A-Za-z0-9]+$")) and
     (map(.ruleId | ascii_downcase) | unique | length) == length then . else error("Unsafe or duplicate rule IDs") end
' "$benchmark_project"/benchmark/*/specification.json > "$benchmark_output/specifications.tmp.json"
jq --arg revision "$revision" '{revision: $revision,
  clusterToRule: (map({key: (.ruleId | ascii_downcase), value: .ruleId}) | from_entries), specifications: .}' \
  "$benchmark_output/specifications.tmp.json" > "$benchmark_output/inputs.json"
mkdir -p "$benchmark_output/state/inbox" "$benchmark_output/state/inspections"
while IFS= read -r specification; do
  rule=$(jq -r '.ruleId' <<< "$specification")
  cluster=$(jq -r '.ruleId | ascii_downcase' <<< "$specification")
  mkdir -p "$benchmark_output/state/clusters/$cluster"
  jq --arg id "$cluster" '{id: $id, description, language, status: "Pending"}' <<< "$specification" \
    > "$benchmark_output/state/clusters/$cluster/description.json"
  printf '# Submitted feedback\n\nRequired labelled examples from benchmark/%s/specification.json.\n' "$rule" \
    > "$benchmark_output/state/clusters/$cluster/history.md"
  while IFS= read -r example; do
    field=$(jq -r '.field' <<< "$example")
    index=$(jq -r '.index' <<< "$example")
    path=$(jq -r '.value.path' <<< "$example")
    source_revision=$(jq -r --arg revision "$revision" '.value.revision | if . == null or . == "" then $revision else . end' <<< "$example")
    line_count=$(git -C "$benchmark_project" show "$source_revision:$path" | awk 'END {print NR}')
    origin="benchmark/$rule/specification.json#/$field/$index"
    key="benchmark:$revision:$origin"
    signal_id="s-$(printf '%s' "$key" | shasum -a 256 | cut -c 1-10)"
    jq --arg id "$signal_id" --arg key "$key" --arg revision "$source_revision" --arg origin "$origin" \
      --arg url "git:$revision:$origin" --argjson lineCount "$line_count" --argjson spec "$specification" '
      (.value.expectedProblemRanges // [] | if length == 0 then [{start:1,end:$lineCount}] else . end) as $ranges |
      if all($ranges[]; .start >= 1 and .end >= .start and .start <= $lineCount) then . else error("Invalid required range") end |
      {id: $id, idempotencyKey: $key, fileRevision: {path: .value.path, revision: $revision, expectedRanges: $ranges},
       source: {type:"SubmittedFeedback", diffPositiveToNegative:"", message:$spec.description, url:$url},
       label: (if .field == "positiveExamples" then "POSITIVE" else "NEGATIVE" end),
       description: ($spec.description | gsub("\\s+"; " ") | .[:1000]), provenance: {workItemId:$origin}}' \
      <<< "$example" > "$benchmark_output/state/inbox/$signal_id.json"
  done < <(jq -c 'to_entries[] | select(.key == "positiveExamples" or .key == "negativeExamples") |
    .key as $field | .value | to_entries[] | {field:$field,index:.key,value:.value}' <<< "$specification")
done < <(jq -c '.[]' "$benchmark_output/specifications.tmp.json")
rm "$benchmark_output/specifications.tmp.json"
printf '%s\n' "$revision" > "$benchmark_output/source-revision.txt"
echo "Imported $(find "$benchmark_output/state/inbox" -name '*.json' | wc -l) required signals."
