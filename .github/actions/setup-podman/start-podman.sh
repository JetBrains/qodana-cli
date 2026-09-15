#!/usr/bin/env bash
set -euo pipefail

socket_timeout=60
api_timeout=60

service_log=$(mktemp "${RUNNER_TEMP:-/tmp}/podman-service.XXXXXX") \
  || { echo "::error::could not create a log file under ${RUNNER_TEMP:-/tmp}"; exit 1; }
probe_log=$(mktemp "${RUNNER_TEMP:-/tmp}/docker-probe.XXXXXX") \
  || { echo "::error::could not create a log file under ${RUNNER_TEMP:-/tmp}"; exit 1; }

fail() {
  echo "::error::$1"
  echo "--- podman system service log ---"; cat "${service_log}" || true
  if [[ -s "${probe_log}" ]]; then
    echo "--- last docker probe output ---"; cat "${probe_log}" || true
  fi
  echo "--- podman processes ---"; pgrep -a podman || echo "(none)"
  exit 1
}

podman_version=$(sudo podman version --format="{{.Server.Version}}") \
  || fail "could not determine the podman server version"
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
      (( SECONDS < stale_deadline )) \
        || fail "a previous podman service on ${podman_socket} would not exit"
      sleep 0.1
    done
    ;;
  1) : ;;  # nothing matched, which is the normal case
  *) fail "pkill failed looking for a previous podman service (exit ${pkill_status})" ;;
esac
sudo rm -f "${podman_socket}" || fail "could not remove a stale ${podman_socket}"

sudo podman system service --time=0 "unix://${podman_socket}" >"${service_log}" 2>&1 &
service_pid=$!

deadline=$(( SECONDS + socket_timeout ))
until [[ -S "${podman_socket}" ]]; do
  kill -0 "${service_pid}" 2>/dev/null || [[ -S "${podman_socket}" ]] \
    || fail "podman system service exited before creating ${podman_socket}."
  (( SECONDS < deadline )) \
    || fail "podman system service did not create ${podman_socket} within ${socket_timeout}s."
  sleep 0.1
done

sudo chown "$(id -u):$(id -g)" "${podman_socket}"

# docker context rm refuses to remove the context currently in use.
docker context use default >/dev/null \
  || fail "could not switch to the default docker context"
if docker context inspect "${context_name}" >/dev/null 2>&1; then
  docker context rm -f "${context_name}" >/dev/null \
    || fail "could not remove the existing ${context_name} docker context"
fi
docker context create --docker "host=unix://${podman_socket}" "${context_name}" \
  || fail "could not create the ${context_name} docker context"
docker context use "${context_name}" \
  || fail "could not select the ${context_name} docker context"

# docker ps exits 1 (not 124) when it can't reach the daemon, and `timeout`
# passes the child's status through verbatim, so every status is retried.
deadline=$(( SECONDS + api_timeout ))
until timeout -k 1 5 docker ps >"${probe_log}" 2>&1; do
  kill -0 "${service_pid}" 2>/dev/null \
    || fail "podman system service died before serving the API"
  (( SECONDS < deadline )) \
    || fail "podman API on ${podman_socket} was unreachable for ${api_timeout}s"
  sleep 0.5
done

# The docker CLI resolves DOCKER_HOST ahead of the selected context, so a job
# with it set would otherwise silently talk to the runner's own dockerd.
server_version=$(docker version --format '{{.Server.Version}}') \
  || fail "docker version failed against ${podman_socket}"
if [[ "${server_version}" != "${podman_version}" ]]; then
  fail "docker CLI is served by ${server_version}, not the podman ${podman_version} started here"
fi

echo "--- podman system service startup log ---"; cat "${service_log}" || true
echo "docker CLI is served by podman ${server_version}"
