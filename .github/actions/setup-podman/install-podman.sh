#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != Linux ]]; then
  echo "::error::setup-podman supports Linux runners only (got $(uname -s))"
  exit 1
fi

command -v docker >/dev/null \
  || { echo "::error::setup-podman needs a preinstalled docker CLI"; exit 1; }

# Ubuntu noble ships crun 1.14.1, below the 1.14.3 podman needs.
CRUN_MIN_VERSION="1.14.3"

# VERSION is attacker-influenced via workflow_dispatch and reaches a download URL.
if [[ "${VERSION}" != "latest" && ! "${VERSION}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::podman needs a full major.minor.patch version or 'latest', got '${VERSION}'"
  exit 1
fi

BASE_URL="https://github.com/mgoltzsche/podman-static/releases/latest/download"
if [[ "${VERSION}" != "latest" ]]; then
  BASE_URL="https://github.com/mgoltzsche/podman-static/releases/download/v${VERSION#v}"
fi

ARCHIVE_NAME="podman-linux-$(dpkg --print-architecture)"
ARCHIVE_FILE="/tmp/${ARCHIVE_NAME}.tar.gz"

curl -fSL --retry 3 --retry-delay 5 "${BASE_URL}/${ARCHIVE_NAME}.tar.gz" -o "${ARCHIVE_FILE}"

if ! gzip -t "${ARCHIVE_FILE}" 2>/dev/null; then
  echo "::error::downloaded file is not a valid gzip archive"
  file "${ARCHIVE_FILE}"
  exit 1
fi

cd /tmp
rm -rf "/tmp/${ARCHIVE_NAME}"
tar -xzf "${ARCHIVE_FILE}"
for required in usr/local/bin/podman usr/local/bin/crun; do
  if [[ ! -x "${ARCHIVE_NAME}/${required}" ]]; then
    echo "::error::the podman-static tarball has no ${required}"
    exit 1
  fi
done
sudo rsync -av "${ARCHIVE_NAME}/etc/" /etc
sudo rsync -av "${ARCHIVE_NAME}/usr/" /usr

# podman resolves crun from /usr/bin/crun before the bundled /usr/local/bin/crun.
sudo install -m 0755 "${ARCHIVE_NAME}/usr/local/bin/crun" /usr/bin/crun

resolved_podman=$(sudo sh -c 'command -v podman') \
  || { echo "::error::sudo cannot resolve podman at all"; exit 1; }
sudo cmp -s "${resolved_podman}" "${ARCHIVE_NAME}/usr/local/bin/podman" || case $? in
  1) echo "::error::sudo resolves podman to ${resolved_podman}, which is not the binary just installed"
     exit 1 ;;
  *) echo "::error::could not compare ${resolved_podman} with the extracted podman binary"
     exit 1 ;;
esac

sudo podman version

podman_version=$(sudo podman version --format="{{.Server.Version}}") \
  || { echo "::error::could not determine the podman server version"; exit 1; }
if [[ "${VERSION}" != "latest" && "${podman_version}" != "${VERSION#v}" ]]; then
  echo "::error::requested podman ${VERSION#v} but ${podman_version} is what runs"
  exit 1
fi

runtime_info=$(sudo podman info --format '{{.Host.OCIRuntime.Name}}|{{.Host.OCIRuntime.Path}}|{{.Host.OCIRuntime.Version}}') \
  || { echo "::error::podman info failed; the engine cannot describe its runtime"; exit 1; }
runtime_name="${runtime_info%%|*}"
runtime_rest="${runtime_info#*|}"
runtime_path="${runtime_rest%%|*}"
runtime_version="${runtime_rest#*|}"
echo "podman resolved OCI runtime: ${runtime_name} at ${runtime_path}"
echo "${runtime_version}"

crun_version_re='crun version ([0-9]+\.[0-9]+(\.[0-9]+)?)'
if [[ "${runtime_name}" != crun ]]; then
  echo "::error::podman resolved ${runtime_name} at ${runtime_path}, not the bundled crun"
  exit 1
fi
if [[ ! "${runtime_version}" =~ $crun_version_re ]]; then
  echo "::error::could not read a crun version from podman info"
  exit 1
fi
if ! printf '%s\n%s\n' "${CRUN_MIN_VERSION}" "${BASH_REMATCH[1]}" | sort -V -C; then
  echo "::error::crun ${BASH_REMATCH[1]} is older than the required ${CRUN_MIN_VERSION}"
  exit 1
fi
