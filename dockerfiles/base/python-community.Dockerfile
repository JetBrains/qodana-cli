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
# emSign ECC Root CA - G3 and AAA Certificate Services roots, so they are upgraded too (QD-16607). Python 3.14's
# ensurepip bundles pip 26.2.2 and virtualenv seeds Python 3.10 and later with pip 26.3, whose certifi bundles no longer
# have them. virtualenv's pip 26.0.1 for Python 3.9, which is end-of-life and has no later pip, still has emSign ECC G3,
# so it is removed: virtualenv can no longer seed Python 3.9 environments. The removal fails once virtualenv drops it.
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
      "ca-certificates>=2026.10.19" "certifi>=2026.10.19" "pip>=26.2.2" "virtualenv>=21.15.6" "python>=3.14.10,<3.15" && \
    ${CONDA_DIR}/bin/conda clean -afy && \
    "$CONDA_DIR/bin/python" -I -c "import os, virtualenv.seed.wheels.embed as embed; os.remove(os.path.join(embed.BUNDLE_FOLDER, 'pip-26.0.1-py3-none-any.whl'))" && \
    poetry config virtualenvs.create false && \
    chmod 777 -R $HOME/.conda $CONDA_DIR/ $HOME/.config/pypoetry/ && \
    rm -rf /tmp/*
