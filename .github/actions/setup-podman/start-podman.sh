#!/usr/bin/env bash
set -euxo pipefail

# Both streams land in the same job log; merge them so the trace and the command
# output below stay in order.
exec 2>&1

socket_timeout=60
api_timeout=60

if ! service_log=$(mktemp "${RUNNER_TEMP:-/tmp}/podman-service.XXXXXX"); then
  echo "::error::could not create a log file under ${RUNNER_TEMP:-/tmp}" >&2
  exit 1
fi
if ! probe_log=$(mktemp "${RUNNER_TEMP:-/tmp}/docker-probe.XXXXXX"); then
  echo "::error::could not create a log file under ${RUNNER_TEMP:-/tmp}" >&2
  exit 1
fi

fail() {
  echo "::error::$1" >&2
  echo "--- podman system service log ---" >&2
  cat "${service_log}" >&2 || true
  if [[ -s "${probe_log}" ]]; then
    echo "--- last docker probe output ---" >&2
    cat "${probe_log}" >&2 || true
  fi
  echo "--- podman processes ---" >&2
  pgrep -a podman >&2 || echo "(none)" >&2
  exit 1
}

if ! podman_version=$(sudo podman version --format="{{.Server.Version}}"); then
  fail "could not determine the podman server version"
fi
podman_socket="/var/run/podman-${podman_version}.sock"
context_name="podman-${podman_version}"

# Stop a previous invocation's service (it never exits on its own: --time=0),
# then remove its socket -- a leftover file would satisfy the wait below
# immediately and the chown would land on an inode podman then replaces.
# [p] keeps this pattern from matching the `sudo pkill` invocation itself.
stale_service_re="podman system service .*/var/run/[p]odman-${podman_version//./\\.}\.sock"
pkill_status=0
sudo pkill -f "${stale_service_re}" || pkill_status=$?
case "${pkill_status}" in
  0)
    stale_deadline=$(( SECONDS + 30 ))
    while sudo pgrep -f "${stale_service_re}" >/dev/null; do
      if (( SECONDS >= stale_deadline )); then
        fail "a previous podman service on ${podman_socket} would not exit"
      fi
      sleep 0.1
    done
    ;;
  1) : ;;  # nothing matched, which is the normal case
  *) fail "pkill failed looking for a previous podman service (exit ${pkill_status})" ;;
esac

if ! sudo rm -f "${podman_socket}"; then
  fail "could not remove a stale ${podman_socket}"
fi

sudo podman system service --time=0 "unix://${podman_socket}" >"${service_log}" 2>&1 &
service_pid=$!

deadline=$(( SECONDS + socket_timeout ))
until [[ -S "${podman_socket}" ]]; do
  if ! kill -0 "${service_pid}" 2>/dev/null && [[ ! -S "${podman_socket}" ]]; then
    fail "podman system service exited before creating ${podman_socket}"
  fi
  if (( SECONDS >= deadline )); then
    fail "podman system service did not create ${podman_socket} within ${socket_timeout}s"
  fi
  sleep 0.1
done

sudo chown "$(id -u):$(id -g)" "${podman_socket}"

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

# docker ps exits 1 (not 124) when it can't reach the daemon, and `timeout`
# passes the child's status through verbatim, so every status is retried.
deadline=$(( SECONDS + api_timeout ))
until timeout -k 1 5 docker ps >"${probe_log}" 2>&1; do
  if ! kill -0 "${service_pid}" 2>/dev/null; then
    fail "podman system service died before serving the API"
  fi
  if (( SECONDS >= deadline )); then
    fail "podman API on ${podman_socket} was unreachable for ${api_timeout}s"
  fi
  sleep 0.5
done

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
