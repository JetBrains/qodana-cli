#!/usr/bin/env bash
set -euxo pipefail

# Both streams land in the same job log; merge them so the trace and the command
# output below stay in order.
exec 2>&1

if [[ "$(uname -s)" != Linux ]]; then
  echo "::error::setup-podman supports Linux runners only (got $(uname -s))" >&2
  exit 1
fi

if ! command -v docker >/dev/null; then
  echo "::error::setup-podman needs a preinstalled docker CLI" >&2
  exit 1
fi

# Ubuntu noble ships crun 1.14.1, below the 1.14.3 podman needs.
crun_min_version="1.14.3"

# VERSION reaches a download URL below, so check its shape first.
if [[ "${VERSION}" != "latest" && ! "${VERSION}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::podman needs a full major.minor.patch version or 'latest', got '${VERSION}'" >&2
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
  echo "::error::downloaded file is not a valid gzip archive" >&2
  file "${ARCHIVE_FILE}"
  exit 1
fi

cd /tmp
rm -rf "/tmp/${ARCHIVE_NAME}"
tar -xzf "${ARCHIVE_FILE}"

for required in usr/local/bin/podman usr/local/bin/crun; do
  if [[ ! -x "${ARCHIVE_NAME}/${required}" ]]; then
    echo "::error::the podman-static tarball has no ${required}" >&2
    exit 1
  fi
done

sudo rsync -av "${ARCHIVE_NAME}/etc/" /etc
sudo rsync -av "${ARCHIVE_NAME}/usr/" /usr

# podman resolves crun from /usr/bin/crun before the bundled /usr/local/bin/crun.
sudo install -m 0755 "${ARCHIVE_NAME}/usr/local/bin/crun" /usr/bin/crun

if ! resolved_podman=$(sudo sh -c 'command -v podman'); then
  echo "::error::sudo cannot resolve podman at all" >&2
  exit 1
fi

# cmp exits 1 for "differs" and 2 for an I/O error; only the first is a mismatch.
cmp_status=0
sudo cmp -s "${resolved_podman}" "${ARCHIVE_NAME}/usr/local/bin/podman" || cmp_status=$?
if (( cmp_status == 1 )); then
  echo "::error::sudo resolves podman to ${resolved_podman}, which is not the binary just installed" >&2
  exit 1
fi
if (( cmp_status != 0 )); then
  echo "::error::could not compare ${resolved_podman} with the extracted podman binary" >&2
  exit 1
fi

sudo podman version

if ! podman_version=$(sudo podman version --format="{{.Server.Version}}"); then
  echo "::error::could not determine the podman server version" >&2
  exit 1
fi

if [[ "${VERSION}" != "latest" && "${podman_version}" != "${VERSION#v}" ]]; then
  echo "::error::requested podman ${VERSION#v} but ${podman_version} is what runs" >&2
  exit 1
fi

if ! runtime_info=$(sudo podman info --format '{{.Host.OCIRuntime.Name}}|{{.Host.OCIRuntime.Path}}|{{.Host.OCIRuntime.Version}}'); then
  echo "::error::podman info failed; the engine cannot describe its runtime" >&2
  exit 1
fi

runtime_name="${runtime_info%%|*}"
runtime_rest="${runtime_info#*|}"
runtime_path="${runtime_rest%%|*}"
runtime_version="${runtime_rest#*|}"
echo "podman resolved OCI runtime: ${runtime_name} at ${runtime_path}"
echo "${runtime_version}"

crun_version_re='crun version ([0-9]+\.[0-9]+(\.[0-9]+)?)'
if [[ "${runtime_name}" != crun ]]; then
  echo "::error::podman resolved ${runtime_name} at ${runtime_path}, not the bundled crun" >&2
  exit 1
fi
if [[ ! "${runtime_version}" =~ $crun_version_re ]]; then
  echo "::error::could not read a crun version from podman info" >&2
  exit 1
fi
if ! printf '%s\n%s\n' "${crun_min_version}" "${BASH_REMATCH[1]}" | sort -V -C; then
  echo "::error::crun ${BASH_REMATCH[1]} is older than the required ${crun_min_version}" >&2
  exit 1
fi
