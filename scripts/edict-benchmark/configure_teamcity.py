#!/usr/bin/env python3
"""Update the dedicated benchmark configuration using the authenticated TeamCity CLI.

Create it by copying the reference build first, so TeamCity retains its secure
Qodana and LiteLLM parameters without exposing them to this script.
"""
import base64
import json
import re
from pathlib import Path
import subprocess
import tempfile

JOB = "StaticAnalysis_Edict_Benchmarks_JenkinsKotlinSkills"
PROJECT = "StaticAnalysis_Edict_Benchmarks"
REFERENCE = "StaticAnalysis_Tests_Benchmarks_EdictGenerationBenchmark_jenkins_codex_skill_NewBench"
IMAGE = "registry.jetbrains.team/p/sa/containers/qodana-jvm:263.SNAPSHOT.149"
ROOT = Path(__file__).resolve().parent


def api(path, value, method="PUT"):
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as payload:
        json.dump(value, payload)
        payload.flush()
        subprocess.run(["teamcity", "api", path, "-X", method, "--input", payload.name, "--silent"], check=True)


def properties(values):
    return {"property": [{"name": key, "value": value} for key, value in values.items()]}


def step(identity, name, script, mode="default"):
    return {"id": identity, "name": name, "type": "simpleRunner", "properties": properties({
        "script.content": script, "use.custom.script": "true", "teamcity.step.mode": mode})}


def main():
    # An explicit create flag avoids treating authentication/network failures as "not found".
    import argparse
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--create", action="store_true")
    parser.add_argument("--comparison-revision", required=True,
                        help="Published qodana-cli commit containing edict/kotlin/benchmark (full SHA)")
    args = parser.parse_args()
    if not re.fullmatch(r"[0-9a-f]{40}", args.comparison_revision):
        parser.error("--comparison-revision must be a full commit SHA")
    if args.create:
        api(f"/app/rest/projects/id:{PROJECT}/buildTypes", {"id": JOB, "name": "Jenkins: Kotlin managed skills",
            "sourceBuildType": {"id": REFERENCE}, "copyAllAssociatedSettings": True}, "POST")
    payload = {name: base64.b64encode((ROOT / name).read_bytes()).decode()
               for name in ("benchmark.py", "BenchmarkHost.java", "run.sh", "generate.sh", "compare.sh")}
    prepare = """#!/usr/bin/env bash
set -euo pipefail
test -n "${LITELLM_API_KEY:-}"
test -n "${QODANA_TOKEN:-}"
test -s edict-runtime/edict-cli.jar
test -s project/benchmark/gold.sarif.json
mkdir -p benchmark-scripts benchmark-output
python3 - <<'PYTHON'
import base64, json
from pathlib import Path
payload = json.loads(r'''PAYLOAD''')
for name, content in payload.items():
    Path('benchmark-scripts', name).write_bytes(base64.b64decode(content))
PYTHON
docker pull "%benchmark.image%"
docker image inspect "%benchmark.image%" --format '{{json .RepoDigests}}' > benchmark-output/image-digests.json
git -C project rev-parse HEAD > benchmark-output/source-revision.txt
sha256sum edict-runtime/edict-cli.jar > benchmark-output/edict-jar.sha256
""".replace("PAYLOAD", json.dumps(payload))
    api(f"/app/rest/buildTypes/id:{JOB}/steps", {"step": [
        step("PREPARE_KOTLIN", "Prepare pinned Kotlin runtime and assembled Qodana image", prepare),
        step("KOTLIN_BENCHMARK", "Start inspections MCP and generate with Kotlin managed skills",
             '#!/bin/bash\nset -euo pipefail\nexport BENCHMARK_CONTAINER="edict-benchmark-%teamcity.build.id%"\nbash benchmark-scripts/generate.sh\n'),
        step("STOP_BENCHMARK", "Stop benchmark container", '#!/bin/bash\ndocker rm -f "edict-benchmark-%teamcity.build.id%" >/dev/null 2>&1 || true\n', "execute_always"),
        step("COMPARE_KOTLIN", "Checkout benchmark comparison and run Gradle :benchmark:compare",
             '#!/bin/bash\nset -euo pipefail\nbash benchmark-scripts/compare.sh\n', "execute_always")
    ]})
    # Consume the already assembled image and JAR, without triggering expensive upstream rebuilds.
    api(f"/app/rest/buildTypes/id:{JOB}/snapshot-dependencies", {"snapshot-dependency": []})
    api(f"/app/rest/buildTypes/id:{JOB}/artifact-dependencies", {"artifact-dependency": [{
        "id": "EDICT_JAR", "type": "artifact_dependency", "source-buildType": {"id": "StaticAnalysis_Edict_BuildAndPublish"},
        "properties": properties({"pathRules": "edict-cli.jar => edict-runtime", "cleanDestinationDirectory": "true",
                                  "revisionName": "buildId", "revisionValue": "1070533116"})}]})
    parameters = {"benchmark.image": IMAGE, "env.BENCHMARK_IMAGE": "%benchmark.image%",
                  "env.BENCHMARK_COMPARISON_REVISION": args.comparison_revision,
                  "env.LITELLM_API_KEY": "%liteLLMToken%",
                  "env.BENCHMARK_MODEL": "gpt-5.6-sol", "env.BENCHMARK_MINUTES": "240",
                  "env.BENCHMARK_LIMIT": "0", "env.BENCHMARK_RULES": "", "env.BENCHMARK_PREFLIGHT": "false",
                  "env.BENCHMARK_CODEX_VERSION": "0.155.1"}
    for name, value in parameters.items():
        api(f"/app/rest/buildTypes/id:{JOB}/parameters", {"name": name, "value": value}, "POST")
    settings = {"executionTimeoutMin": "300", "maximumNumberOfBuilds": "1", "cleanBuild": "true",
                "publishArtifactCondition": "ALWAYS", "artifactRules": "\n".join([
                    "benchmark-output/report.json", "benchmark-output/progress.json", "benchmark-output/qodana.sarif.json", "benchmark-output/inputs.json",
                    "benchmark-output/last-message.txt", "benchmark-output/inspection-tools.json",
                    "benchmark-output/image-digests.json", "benchmark-output/source-revision.txt", "benchmark-output/edict-jar.sha256",
                    "benchmark-output/comparison-revision.txt", "benchmark-output/generatedInspections => generatedInspections.zip",
                    "benchmark-output/specGoldComparisons => specGoldComparisons.zip",
                    "benchmark-output/state => state.zip", "benchmark-output/log => logs.zip",
                    "benchmark-output/trace/sandbox.stderr", "benchmark-output/trace/sandbox.stdout",
                    "benchmark-output/mcp-results/log => inspection-ide-logs.zip",
                    "benchmark-output/evaluation/results => evaluation.zip",
                    "benchmark-output/project-runs/**/analysis.log => project-analysis-logs.zip",
                    "benchmark-output/evaluation/analysis.log", "benchmark-scripts => runner.zip"
                ])}
    for name, value in settings.items():
        api(f"/app/rest/buildTypes/id:{JOB}/settings/{name}", {"name": name, "value": value})
    current = json.loads(subprocess.check_output(["teamcity", "api", f"/app/rest/buildTypes/id:{JOB}/parameters"], text=True))
    current_names = {parameter["name"] for parameter in current.get("property", [])}
    for name in ("dockerImageForBenchmark", "forceLoadFromDependency", "genIterationsCount",
                 "qodanaCliBenchmarkArgs", "edict.bench.limit.scope", "pushToVcs", "vcs.project.branch"):
        # These belonged to the copied in-IDE generator and are not controls for managed skills.
        if name in current_names:
            subprocess.run(["teamcity", "api", f"/app/rest/buildTypes/id:{JOB}/parameters/{name}",
                            "-X", "DELETE", "--silent"], check=True)
    print(f"https://buildserver.labs.intellij.net/buildConfiguration/{JOB}")


if __name__ == "__main__":
    main()
