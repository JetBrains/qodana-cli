ARG BASE_TAG="bullseye-slim"
ARG NODE_TAG="20-bullseye-slim"
FROM node:$NODE_TAG AS node_base
FROM debian:$BASE_TAG

# renovate: datasource=npm depName=eslint
ENV ESLINT_VERSION="9.8.0"
# renovate: datasource=npm depName=pnpm
ENV PNPM_VERSION="9.6.0"

ENV HOME="/root" \
    LC_ALL="en_US.UTF-8" \
    QODANA_DIST="/opt/idea" \
    QODANA_DATA="/data" \
    QODANA_DOCKER="true"
ENV JAVA_HOME="$QODANA_DIST/jbr" \
    QODANA_CONF="$HOME/.config/idea" \
    PATH="$QODANA_DIST/bin:$PATH"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    rm -f /etc/apt/apt.conf.d/docker-clean && \
    mkdir -m 777 -p /opt $QODANA_DATA $QODANA_CONF && apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        fontconfig \
        git \
        git-lfs \
        gnupg2 \
        locales \
        procps && \
    echo 'en_US.UTF-8 UTF-8' > /etc/locale.gen && locale-gen && \
    apt-get autoremove -y && apt-get clean && \
    chmod 777 -R $HOME && \
    echo 'root:x:0:0:root:/root:/bin/bash' > /etc/passwd && chmod 666 /etc/passwd && \
    git config --global --add safe.directory '*'

ENV CONDA_DIR="/opt/miniconda3" \
    CONDA_ENVS_PATH="$QODANA_DATA/cache/conda/envs" \
    PIP_CACHE_DIR="$QODANA_DATA/cache/.pip/" \
    POETRY_CACHE_DIR="$QODANA_DATA/cache/.poetry/" \
    FLIT_ROOT_INSTALL=1
ENV PATH="$CONDA_DIR/bin:$HOME/.local/bin:$PATH"

# https://docs.conda.io/projects/miniconda/en/latest/miniconda-hashes.html
ARG CONDA_VERSION="py311_24.5.0-0"

# hadolint ignore=SC2174,DL3009
RUN --mount=target=/var/lib/apt/lists,type=cache,sharing=locked \
    --mount=target=/var/cache/apt,type=cache,sharing=locked \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      bzip2 \
      libglib2.0-0 \
      libsm6 \
      libxext6 \
      libxrender1 && \
    mkdir -m 777 -p $QODANA_DATA/cache && \
    dpkgArch="$(dpkg --print-architecture)" && \
    case "$dpkgArch" in \
      'amd64')  \
        MINICONDA_URL="https://repo.anaconda.com/miniconda/Miniconda3-${CONDA_VERSION}-Linux-x86_64.sh" \
        SHA256SUM="38b203bb1f2be78b735ebc00162f29e8e73fcd9a619ed5980490a72193ee1f58";; \
      'arm64')  \
        MINICONDA_URL="https://repo.anaconda.com/miniconda/Miniconda3-${CONDA_VERSION}-Linux-aarch64.sh"  \
        SHA256SUM="94a742af7bf5c7bae3dba6bd07d84d94b858b839e15af2ea0cd10fdf2bde8a73";; \
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
    poetry config virtualenvs.create false && \
    chmod 777 -R $HOME/.conda $CONDA_DIR/ $HOME/.config/pypoetry/ && \
    rm -rf /tmp/*

ENV PATH="/opt/yarn/bin:$PATH"
ENV SKIP_YARN_COREPACK_CHECK=0
COPY --from=node_base /usr/local/bin/node /usr/local/bin/
COPY --from=node_base /usr/local/include/node /usr/local/include/node
COPY --from=node_base /usr/local/lib/node_modules /usr/local/lib/node_modules
COPY --from=node_base /opt/yarn-* /opt/yarn/
RUN ln -s /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm && \
    ln -s /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx && \
    ln -s /usr/local/lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack && \
    node --version && \
    npm --version && \
    yarn --version && \
    npm install -g eslint@$ESLINT_VERSION pnpm@$PNPM_VERSION && npm config set update-notifier false && \
    chmod 777 -R "$HOME/.npm" "$HOME/.npmrc"

ARG QD_RELEASE="2024.2"
ARG QD_BUILD="QDPY-$QD_RELEASE"
# hadolint ignore=DL3003,SC2043
RUN set -ex && \
    dpkgArch="$(dpkg --print-architecture)" && \
    case "$dpkgArch" in \
        "amd64") \
            OS_ARCH_SUFFIX=""; \
            ;; \
        "arm64") \
            OS_ARCH_SUFFIX="-aarch64"; \
            ;; \
        *) echo "Unsupported architecture $dpkgArch" >&2; exit 1 ;; \
    esac && \
    QD_NAME="qodana-$QD_BUILD$OS_ARCH_SUFFIX" \
    QD_URL="https://download.jetbrains.com/qodana/$QD_RELEASE/$QD_NAME.tar.gz" && \
    curl -fsSL "$QD_URL" -o "/tmp/$QD_NAME.tar.gz" \
               "$QD_URL.sha256" -o "/tmp/$QD_NAME.tar.gz.sha256" \
               "$QD_URL.sha256.asc" -o "/tmp/$QD_NAME.tar.gz.sha256.asc" && \
    GNUPGHOME="$(mktemp -d)" && \
    export GNUPGHOME && \
    for key in \
        "B46DC71E03FEEB7F89D1F2491F7A8F87B9D8F501" \
      ; do \
        gpg --batch --keyserver "hkps://keys.openpgp.org" --recv-keys "$key" || \
        gpg --batch --keyserver "keyserver.ubuntu.com" --recv-keys "$key" ; \
    done && \
    gpg --verify "/tmp/$QD_NAME.tar.gz.sha256.asc" "/tmp/$QD_NAME.tar.gz.sha256" && \
    (cd /tmp && sha256sum --check --status "$QD_NAME.tar.gz.sha256") && \
    mkdir -p /tmp/qd && tar -xzf "/tmp/$QD_NAME.tar.gz" --directory /tmp/qd  && \
    mv /tmp/qd/qodana-QD* "$QODANA_DIST" && \
    chmod +x "$QODANA_DIST"/bin/*.sh "$QODANA_DIST"/bin/qodana && \
    update-alternatives --install /usr/bin/java java "$JAVA_HOME/bin/java" 0 && \
    update-alternatives --install /usr/bin/javac javac "$JAVA_HOME/bin/javac" 0 && \
    update-alternatives --set java "$JAVA_HOME/bin/java" && \
    update-alternatives --set javac "$JAVA_HOME/bin/javac" && \
    apt-get purge --auto-remove -y gnupg2 && \
    rm -rf /var/cache/apt /var/lib/apt/ /tmp/* "$GNUPGHOME"

LABEL maintainer="qodana-support@jetbrains.com" description="Qodana for Python (https://jb.gg/qodana-python)"
WORKDIR /data/project
ENTRYPOINT ["/opt/idea/bin/qodana"]