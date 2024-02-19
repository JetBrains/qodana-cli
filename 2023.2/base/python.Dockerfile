FROM debianbase

ENV CONDA_DIR="/opt/miniconda3" \
    CONDA_ENVS_PATH="$QODANA_DATA/cache/conda/envs" \
    PIP_CACHE_DIR="$QODANA_DATA/cache/.pip/" \
    POETRY_CACHE_DIR="$QODANA_DATA/cache/.poetry/" \
    FLIT_ROOT_INSTALL=1
ENV PATH="$CONDA_DIR/bin:$HOME/.local/bin:$PATH"

# https://docs.conda.io/projects/miniconda/en/latest/miniconda-hashes.html
ARG CONDA_VERSION="py310_22.11.1-1"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bzip2=1.0.8-4 \
      libglib2.0-0=2.66.8-1+deb11u1 \
      libsm6=2:1.2.3-1 \
      libxext6=2:1.3.3-1.1 \
      libxrender1=1:0.9.10-1 && \
    mkdir -m 777 -p $QODANA_DATA/cache && \
    dpkgArch="$(dpkg --print-architecture)" && \
    case "$dpkgArch" in \
      'amd64')  \
        MINICONDA_URL="https://repo.anaconda.com/miniconda/Miniconda3-${CONDA_VERSION}-Linux-x86_64.sh" \
        SHA256SUM="00938c3534750a0e4069499baf8f4e6dc1c2e471c86a59caa0dd03f4a9269db6";; \
      'arm64')  \
        MINICONDA_URL="https://repo.anaconda.com/miniconda/Miniconda3-${CONDA_VERSION}-Linux-aarch64.sh"  \
        SHA256SUM="48a96df9ff56f7421b6dd7f9f71d548023847ba918c3826059918c08326c2017";; \
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
    ${CONDA_DIR}/bin/conda install -c conda-forge poetry pipenv && ${CONDA_DIR}/bin/conda clean -afy && \
    chmod 777 -R $HOME/.conda && \
    rm -rf /tmp/*