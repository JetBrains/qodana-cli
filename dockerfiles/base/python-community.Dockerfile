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
# emSign ECC Root CA - G3 and AAA Certificate Services roots, so they are upgraded too. virtualenv 21.15.6
# seeds new environments of Python 3.10 and later with pip 26.3, whose certifi no longer has them. Drop that
# once Python bundles pip 26.2.2 or later.
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
    "$CONDA_DIR/bin/python" -I - <<"EOF" && \
    poetry config virtualenvs.create false && \
    chmod 777 -R $HOME/.conda $CONDA_DIR/ $HOME/.config/pypoetry/ && \
    rm -rf /tmp/*
import base64, glob, hashlib, os, re, sys, zipfile

distrusted = {
    "86a1ecba089c4a8d3bbe2734c612ba341d813e043cf9e8a862cd5c57a36bbe6b",
    "d7a7a0fb5d7e2731d771e9484ebcdef71d5f0c3e0a2948782bc83ee0ea699ef4",
}
bundle_name = "pip/_vendor/certifi/cacert.pem"
# A certificate of certifi's bundle: its "# Issuer:" comment lines, the PEM block and the blank line after it
certificate = re.compile(rb"(?:#[^\n]*\n)*-----BEGIN CERTIFICATE-----\n([A-Za-z0-9+/=\n]*?)-----END CERTIFICATE-----\n\n?")
fingerprint = lambda m: hashlib.sha256(base64.b64decode(m.group(1).replace(b"\n", b""))).hexdigest()

wheels = sorted({os.path.realpath(p) for p in glob.glob(os.environ["CONDA_DIR"] + "/lib/python3*/ensurepip/_bundled/pip-*.whl")})
if not wheels:
    sys.exit("No ensurepip pip wheel found, the Miniconda layout has changed")
for wheel in wheels:
    with zipfile.ZipFile(wheel) as source:
        infos = source.infolist()
        contents = {info.filename: source.read(info) for info in infos}
    bundle = contents[bundle_name]
    certificates = list(certificate.finditer(bundle))
    if not certificates or len(certificates) != bundle.count(b"-----BEGIN CERTIFICATE-----"):
        sys.exit(f"Cannot parse the certificates in {bundle_name} of {wheel}")
    removed = [m for m in certificates if fingerprint(m) in distrusted]
    for m in reversed(removed):
        bundle = bundle[:m.start()] + bundle[m.end():]
    if any(fingerprint(m) in distrusted for m in certificate.finditer(bundle)):
        sys.exit(f"Distrusted roots left in {bundle_name} of {wheel}")
    digest = base64.urlsafe_b64encode(hashlib.sha256(bundle).digest()).rstrip(b"=").decode()
    record = next(name for name in contents if name.endswith(".dist-info/RECORD"))
    contents[bundle_name] = bundle
    contents[record], replaced = re.subn(rb"^" + re.escape(bundle_name.encode()) + rb",[^\r\n]*",
                                         f"{bundle_name},sha256={digest},{len(bundle)}".encode(), contents[record], flags=re.M)
    if replaced != 1:
        sys.exit(f"Expected one {bundle_name} line in {record} of {wheel}, found {replaced}")
    with zipfile.ZipFile(wheel + ".new", "w") as target:
        for info in infos:
            target.writestr(info, contents[info.filename])
    os.replace(wheel + ".new", wheel)
    print(f"{wheel}: removed {len(removed)} of {len(certificates)} certificates")
EOF
