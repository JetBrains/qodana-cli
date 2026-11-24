FROM jvm-community

ENV CONDA_DIR="/opt/miniconda3" \
    CONDA_ENVS_PATH="$QODANA_DATA/cache/conda/envs" \
    PIP_CACHE_DIR="$QODANA_DATA/cache/.pip/" \
    POETRY_CACHE_DIR="$QODANA_DATA/cache/.poetry/" \
    FLIT_ROOT_INSTALL=1
ENV PATH="$CONDA_DIR/bin:$HOME/.local/bin:$PATH"

# https://repo.anaconda.com/miniconda/
ARG CONDA_VERSION="py312_24.5.0-0"

# The CA bundles of Miniconda 24.5.0, most notably the certifi vendored by pip 24.0, include the distrusted
# emSign ECC Root CA - G3 and AAA Certificate Services roots, so they are upgraded too (QD-16607). Some pip wheels still
# have them: the one of ensurepip, which seeds every `python -m venv` environment, and the pip 26.0.1 that virtualenv
# seeds Python 3.9 environments with, as no later pip supports 3.9. strip-distrusted-roots.py removes them, and updates
# the hash virtualenv pins for its wheel, which virtualenv then verifies; the IDE's helper pip wheels get the same
# treatment once the IDE is installed.
# Drop it once Python and the IDE bundle pip 26.2.2 or later, and virtualenv no longer seeds Python 3.9.
COPY --from=qodana-tools /strip-distrusted-roots.py /usr/local/lib/qodana/strip-distrusted-roots.py
# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bzip2 \
      libglib2.0-0 \
      libsm6 \
      libxext6 \
      libxrender1 \
      jq && \
    mkdir -m 777 -p $QODANA_DATA/cache && \
    dpkgArch="$(dpkg --print-architecture)" && \
    case "$dpkgArch" in \
      'amd64')  \
        MINICONDA_URL="https://repo.anaconda.com/miniconda/Miniconda3-${CONDA_VERSION}-Linux-x86_64.sh" \
        SHA256SUM="4b3b3b1b99215e85fd73fb2c2d7ebf318ac942a457072de62d885056556eb83e";; \
      'arm64')  \
        MINICONDA_URL="https://repo.anaconda.com/miniconda/Miniconda3-${CONDA_VERSION}-Linux-aarch64.sh"  \
        SHA256SUM="70afe954cc8ee91f605f9aa48985bfe01ecfc10751339e8245eac7262b01298d";; \
      *) echo "Unsupported architecture $TARGETPLATFORM" >&2; exit 1;; \
    esac && \
    curl -fsSL -o /tmp/miniconda.sh "${MINICONDA_URL}" && \
    echo "${SHA256SUM} /tmp/miniconda.sh" > /tmp/shasum && \
    if [ "${CONDA_VERSION}" != "latest" ]; then sha256sum --check --status /tmp/shasum; fi && \
    bash /tmp/miniconda.sh -b -p $CONDA_DIR && \
    ln -s ${CONDA_DIR}/etc/profile.d/conda.sh /etc/profile.d/conda.sh && \
    echo ". ${CONDA_DIR}/etc/profile.d/conda.sh" >> ~/.bashrc && \
    echo "conda activate base" >> ~/.bashrc && ln -s ${CONDA_DIR}/bin/python3 /usr/bin/python3 && \
    find ${CONDA_DIR}/ -follow -type f -name '*.a' -delete && find ${CONDA_DIR}/ -follow -type f -name '*.js.map' -delete && \
    ${CONDA_DIR}/bin/conda install -c conda-forge poetry pipenv \
      "ca-certificates>=2026.10.19" "certifi>=2026.10.19" "pip>=26.2.2" "virtualenv>=21.15.6" "python>=3.12.16,<3.13" \
      "libmambapy<2" && \
    ${CONDA_DIR}/bin/conda clean -afy && \
    "$CONDA_DIR/bin/python" -I /usr/local/lib/qodana/strip-distrusted-roots.py \
      "$CONDA_DIR"/lib/python3*/ensurepip/_bundled/pip-*.whl \
      "$CONDA_DIR"/lib/python3*/site-packages/virtualenv/seed/wheels/embed/pip-*.whl && \
    "$CONDA_DIR/bin/python" -I -c "from virtualenv.seed.wheels.embed import BUNDLE_SUPPORT, get_embed_wheel; [get_embed_wheel(d, v) for v in BUNDLE_SUPPORT for d in BUNDLE_SUPPORT[v]]" && \
    poetry config virtualenvs.create false && \
    chmod 777 -R $HOME/.conda $CONDA_DIR/ $HOME/.config/pypoetry/ && \
    rm -rf /tmp/*
