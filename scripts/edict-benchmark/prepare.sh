#!/usr/bin/env bash
set -euo pipefail
: "${BENCHMARK_SOURCE_REVISION:?}"
: "${BENCHMARK_IMAGE:?}"
: "${LITELLM_API_KEY:?}"
: "${QODANA_TOKEN:?}"
[[ "$BENCHMARK_SOURCE_REVISION" =~ ^[0-9a-f]{40}$ ]]
test -s project/benchmark/gold.sarif.json
export JAVA_HOME="${JDK_21_0:?JDK 21 is required to build the Kotlin runner}"
export PATH="$JAVA_HOME/bin:$PATH"
test "$(git -C runner-source rev-parse HEAD)" = "$BENCHMARK_SOURCE_REVISION"
git -C runner-source rev-parse HEAD > benchmark-output/runner-revision.txt
./runner-source/edict/kotlin/gradlew -p runner-source/edict/kotlin --no-daemon --console=plain :benchmark:runnerJar
mkdir -p benchmark-runtime
cp runner-source/edict/kotlin/benchmark/build/libs/benchmark-runner.jar benchmark-runtime/
sha256sum benchmark-runtime/benchmark-runner.jar > benchmark-output/runner-jar.sha256
docker pull "$BENCHMARK_IMAGE"
docker image inspect "$BENCHMARK_IMAGE" --format '{{json .RepoDigests}}' > benchmark-output/image-digests.json
git -C project rev-parse HEAD > benchmark-output/source-revision.txt
