FROM jvm-community

ENV CONDA_DIR="/opt/miniconda3" \
    CONDA_ENVS_PATH="$QODANA_DATA/cache/conda/envs" \
    PIP_CACHE_DIR="$QODANA_DATA/cache/.pip/" \
    POETRY_CACHE_DIR="$QODANA_DATA/cache/.poetry/" \
    FLIT_ROOT_INSTALL=1
ENV PATH="$CONDA_DIR/bin:$HOME/.local/bin:$PATH"

# https://repo.anaconda.com/miniconda/
ARG CONDA_VERSION="py312_24.5.0-0"

# QD-16607: no CA bundle may trust emSign ECC Root CA - G3 or AAA Certificate Services. Miniconda 24.5.0's do, hence the
# upgrades; Python 3.14's ensurepip (pip 26.2.2) and virtualenv's pip 26.3 are clean. The guard after the install drops
# virtualenv's pip 26.0.1 for end-of-life Python 3.9 (no clean pip exists) and fails the build if a root is left.
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
    "$CONDA_DIR/bin/python" -I - "$CONDA_DIR" <<"EOF" && \
    poetry config virtualenvs.create false && \
    chmod 777 -R $HOME/.conda $CONDA_DIR/ $HOME/.config/pypoetry/ && \
    rm -rf /tmp/*
# QD-16607 guard: drops virtualenv's Python 3.9 pip if present, then fails if any PEM bundle under $CONDA_DIR, or in
# its wheels (pip's certifi), has a distrusted root, or if nothing was checked
import base64, binascii, hashlib, os, re, sys, zipfile

distrusted = {
    "86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b": "emSign ECC Root CA - G3",
    "d7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4": "AAA Certificate Services",
}
python39_pip = os.path.join("virtualenv", "seed", "wheels", "embed", "pip-26.0.1-py3-none-any.whl")
certificate = re.compile(rb"-----BEGIN CERTIFICATE-----([A-Za-z0-9+/=\s]+?)-----END CERTIFICATE-----")
checked, found = 0, []


def check(name, data):
    global checked
    for match in certificate.finditer(data):
        try:
            der = base64.b64decode(b"".join(match.group(1).split()))
        except binascii.Error:
            continue  # not a valid certificate
        checked += 1
        fingerprint = hashlib.sha256(der).hexdigest()
        if fingerprint in distrusted:
            found.append(f"{name}: {distrusted[fingerprint]}")


for folder, _, files in os.walk(sys.argv[1]):  # skips symlinked dirs such as conda's lib/python3.1
    for file in files:
        path = os.path.join(folder, file)
        if os.path.islink(path):
            continue
        if path.endswith(python39_pip):
            os.remove(path)
            print(f"Removed {path}")
        elif file.endswith((".pem", ".crt")):
            with open(path, "rb") as f:
                check(path, f.read())
        elif file.endswith(".whl"):
            with zipfile.ZipFile(path) as wheel:
                for entry in wheel.namelist():
                    if entry.endswith((".pem", ".crt")):
                        check(f"{path}!/{entry}", wheel.read(entry))
if not checked:
    sys.exit(f"No certificates found in {sys.argv[1]}, so nothing was checked")
if found:
    sys.exit("Distrusted root CAs left (QD-16607):\n" + "\n".join(found))
print(f"Checked {checked} certificates in {sys.argv[1]}: no distrusted root CAs")
EOF
