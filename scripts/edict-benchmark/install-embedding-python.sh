#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

# The agent's default Python can be too old for the pinned embedding stack (NumPy 2.2
# requires Python 3.10+). Provision the benchmark environment explicitly so the MCP
# server never resolves dependencies against whichever python3 happens to be first on PATH.
uv="$benchmark_output/tooling/bin/uv"
if [[ ! -x "$uv" ]]; then
  curl -fsSL https://astral.sh/uv/0.8.22/install.sh \
    | env UV_INSTALL_DIR="$benchmark_output/tooling/bin" UV_NO_MODIFY_PATH=1 sh
fi
export UV_PYTHON_INSTALL_DIR="$benchmark_output/tooling/python"
"$uv" python install 3.12
"$uv" venv --python 3.12 --managed-python "$benchmark_embedding_venv"
"$uv" pip install --python "$benchmark_embedding_python" \
  -r "$benchmark_source/edict/kotlin/src/main/resources/edict-next/requirements.txt"

# Loading the embedding stack's OpenMP runtime at Python startup avoids the ARM64
# static-TLS load-order failure without preloading it into the MCP JVM or other steps.
embedding_openmp=$(embedding_libgomp)
[[ -n "$embedding_openmp" ]] || { echo "Embedding environment has no libgomp runtime" >&2; exit 1; }
{
  printf '#!/usr/bin/env bash\n'
  printf 'export LD_PRELOAD=%q\n' "$embedding_openmp${LD_PRELOAD:+:$LD_PRELOAD}"
  printf 'exec %q "$@"\n' "$benchmark_embedding_python"
} > "$benchmark_embedding_launcher"
chmod 755 "$benchmark_embedding_launcher"
"$benchmark_embedding_launcher" -c \
  'import numpy, sentence_transformers; print(f"embedding Python ready: numpy={numpy.__version__}, sentence-transformers={sentence_transformers.__version__}")'
