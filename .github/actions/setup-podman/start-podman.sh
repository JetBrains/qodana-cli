#!/usr/bin/env bash
set -euxo pipefail

# Both streams land in the same job log; merge them so the trace and the command
# output below stay in order.
exec 2>&1

# Failure bound on podman hanging; it spans podman's start-up and first answer.
api_timeout=120
unit=setup-podman
started=""

# systemd opens the log as root, which fs.protected_regular refuses for a
# user-owned file directly in a sticky directory like /tmp.
if ! log_dir=$(mktemp -d "${RUNNER_TEMP:-/tmp}/podman-service.XXXXXX"); then
  echo "::error::could not create a log directory under ${RUNNER_TEMP:-/tmp}" >&2
  exit 1
fi
service_log="${log_dir}/service.log"
probe_log="${log_dir}/probe.log"
: >"${service_log}"

fail() {
  echo "::error::$1" >&2
  # Stopping first makes the state below final: the stop ends only after
  # systemd has reaped podman, which can lag the failure the probe saw.
  if [[ -n "${started}" ]]; then
    sudo systemctl stop "${unit}.socket" "${unit}.service" || true
  fi
  echo "--- podman system service log ---" >&2
  cat "${service_log}" >&2 || true
  if [[ -s "${probe_log}" ]]; then
    echo "--- last API probe output ---" >&2
    cat "${probe_log}" >&2 || true
  fi
  echo "--- podman systemd units ---" >&2
  sudo journalctl --sync || true
  systemctl status --no-pager --full "${unit}.socket" "${unit}.service" >&2 || true
  exit 1
}

if ! podman_version=$(sudo podman version --format="{{.Server.Version}}"); then
  fail "could not determine the podman server version"
fi
if ! podman_bin=$(sudo sh -c 'command -v podman'); then
  fail "sudo cannot resolve podman"
fi
podman_socket="/var/run/podman-${podman_version}.sock"
context_name="podman-${podman_version}"
read -r oom_score_adj </proc/self/oom_score_adj

# systemd-run returns once the socket is bound, owned by us and listening;
# clients queue on it until podman, started by the first one, accepts.
# - StartLimitBurst=1: a podman that dies without accepting would otherwise be
#   re-activated by the queued connection, turning a crash into a hang.
#   The interval must be infinity; 0 disables the limit.
# - Output to a file, not the journal, which ingests asynchronously and could
#   still be catching up when fail() reads it.
# - The step's OOM priority, which the runner raises and podman passes on to containers.
if ! sudo systemd-run --quiet --unit="${unit}" \
    --socket-property=ListenStream="${podman_socket}" \
    --socket-property=SocketUser="${UID}" \
    --socket-property=SocketMode=0600 \
    --property=StartLimitBurst=1 \
    --property=StartLimitIntervalSec=infinity \
    --property=StandardOutput=append:"${service_log}" \
    --property=StandardError=append:"${service_log}" \
    --property=OOMScoreAdjust="${oom_score_adj}" \
    "${podman_bin}" system service --time=0; then
  fail "could not start podman as ${unit} (the action runs once per job)"
fi
started=1

# docker context rm refuses to remove the context currently in use.
if ! docker context use default >/dev/null; then
  fail "could not switch to the default docker context"
fi
if docker context inspect "${context_name}" >/dev/null 2>&1; then
  if ! docker context rm -f "${context_name}" >/dev/null; then
    fail "could not remove the existing ${context_name} docker context"
  fi
fi
if ! docker context create --docker "host=unix://${podman_socket}" "${context_name}"; then
  fail "could not create the ${context_name} docker context"
fi
if ! docker context use "${context_name}"; then
  fail "could not select the ${context_name} docker context"
fi

curl_status=0
curl --unix-socket "${podman_socket}" --max-time "${api_timeout}" -fsS http://podman/_ping >"${probe_log}" 2>&1 || curl_status=$?
case "${curl_status}" in
  0) : ;;
  28) fail "podman API on ${podman_socket} did not answer within ${api_timeout}s" ;;
  *) fail "podman system service did not serve the API on ${podman_socket} (curl exit ${curl_status})" ;;
esac

# The docker CLI resolves DOCKER_HOST ahead of the selected context, so a job
# with it set would otherwise silently talk to the runner's own dockerd.
if ! server_version=$(docker version --format '{{.Server.Version}}'); then
  fail "docker version failed against ${podman_socket}"
fi
if [[ "${server_version}" != "${podman_version}" ]]; then
  fail "docker CLI is served by ${server_version}, not the podman ${podman_version} started here"
fi

echo "--- podman system service startup log ---"
cat "${service_log}" || true
echo "docker CLI is served by podman ${server_version}"
