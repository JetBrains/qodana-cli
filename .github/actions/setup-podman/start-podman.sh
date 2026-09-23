#!/usr/bin/env bash
set -euxo pipefail

# Both streams land in the same job log; merge them so the trace and the command
# output below stay in order.
exec 2>&1

# Failure bounds on podman misbehaving, not synchronisation. The API wait spans
# socket activation, podman's start-up and its first answer.
api_timeout=120
stop_timeout=30

# Each service logs to <log_root>/<unit>/service.log, so a later invocation can
# find a stale service's log from its unit name.
log_root="${RUNNER_TEMP:-/tmp}"
if ! probe_log=$(mktemp "${RUNNER_TEMP:-/tmp}/api-probe.XXXXXX"); then
  echo "::error::could not create a log file under ${RUNNER_TEMP:-/tmp}" >&2
  exit 1
fi

# Result of each previous invocation's service, keyed by unit name without suffix.
declare -A stale_results=()
# The unit systemd-run was asked to create.
started_unit=""

fail() {
  echo "::error::$1" >&2
  # The stop job ends only after systemd has reaped podman, which can lag the
  # failure a client saw; stopping first makes the state dumped below final.
  if [[ -n "${started_unit}" ]]; then
    sudo systemctl stop "${started_unit}.socket" "${started_unit}.service" || true
  fi
  if [[ -n "${service_log:-}" ]]; then
    echo "--- podman system service log ---" >&2
    cat "${service_log}" >&2 || true
  fi
  local base name
  for base in "${!stale_results[@]}"; do
    echo "--- previous podman system service log (${base}) ---" >&2
    cat "${log_root}/${base}/service.log" >&2 || true
  done
  if [[ -s "${probe_log}" ]]; then
    echo "--- last API probe output ---" >&2
    cat "${probe_log}" >&2 || true
  fi
  local -a units=("${!stale_results[@]}")
  if [[ -n "${started_unit}" ]]; then
    units+=("${started_unit}")
  fi
  if (( ${#units[@]} > 0 )); then
    echo "--- podman systemd units ---" >&2
    local -a journal_args=()
    for base in "${units[@]}"; do
      for name in "${base}.socket" "${base}.service"; do
        systemctl show -p Id,ActiveState,SubState,Result,ExecMainStatus "${name}" >&2 || true
        journal_args+=(-u "${name}")
      done
    done
    # systemd's own messages about the units; the service's output is in the logs above.
    sudo journalctl --sync || true
    sudo journalctl --no-pager "${journal_args[@]}" >&2 || true
  fi
  echo "--- podman processes ---" >&2
  pgrep -a podman >&2 || echo "(none)" >&2
  exit 1
}

if ! podman_version=$(sudo podman version --format="{{.Server.Version}}"); then
  fail "could not determine the podman server version"
fi
# It names systemd units below, and systemd would silently escape anything else.
if [[ ! "${podman_version}" =~ ^[0-9A-Za-z._-]+$ ]]; then
  fail "unexpected podman version '${podman_version}'"
fi
if ! podman_bin=$(sudo sh -c 'command -v podman'); then
  fail "sudo cannot resolve podman"
fi
podman_socket="/var/run/podman-${podman_version}.sock"
context_name="podman-${podman_version}"
# The uuid shape keeps 5.8.4 from matching 5.8.4-dev units, which serve another socket.
unit_glob="setup-podman-${podman_version}-????????-????-????-????-????????????"
if ! uuid=$(</proc/sys/kernel/random/uuid); then
  fail "could not generate a unit name"
fi
unit="setup-podman-${podman_version}-${uuid}"
service_log="${log_root}/${unit}/service.log"
# systemd opens the log as root, which fs.protected_regular refuses for a
# user-owned file directly in a sticky directory like /tmp.
if ! mkdir "${log_root}/${unit}" || ! : >"${service_log}"; then
  fail "could not create ${service_log}"
fi

# Stop a previous invocation's service (it never exits on its own: --time=0).
# `systemctl stop` returns when the stop job is done; a service still up
# TimeoutStopSec after SIGTERM is SIGKILLed and left failed with Result=timeout.
if ! stale_listing=$(systemctl list-units --all --plain --no-legend "${unit_glob}.service"); then
  fail "could not list previous podman services"
fi
while read -r service _; do
  [[ -n "${service}" ]] || continue
  if ! stale_results["${service%.service}"]=$(systemctl show -P Result "${service}"); then
    fail "could not read the result of ${service}"
  fi
done <<< "${stale_listing}"
if ! sudo systemctl stop "${unit_glob}.socket" "${unit_glob}.service"; then
  fail "could not stop a previous podman service on ${podman_socket}"
fi
for stale_unit in "${!stale_results[@]}"; do
  if ! result=$(systemctl show -P Result "${stale_unit}.service"); then
    fail "could not read the result of ${stale_unit}.service"
  fi
  # One that timed out in an earlier run already failed that run.
  if [[ "${result}" == timeout && "${stale_results[${stale_unit}]}" != timeout ]]; then
    fail "a previous podman service on ${podman_socket} did not stop within ${stop_timeout}s of SIGTERM"
  fi
done
# Their results are read; unload the failed ones so they don't pile up.
if ! sudo systemctl reset-failed "${unit_glob}.socket" "${unit_glob}.service"; then
  fail "could not reset previous podman units"
fi

# Read here: a failed substitution inside systemd-run's arguments would go unnoticed.
if ! runner_user=$(id -un) || ! runner_group=$(id -gn); then
  fail "could not resolve the runner user and group"
fi
if ! oom_score_adj=$(</proc/self/oom_score_adj); then
  fail "could not read the step's OOM score adjustment"
fi

# systemd-run returns once the socket is bound, owned by us and listening;
# clients queue on it until podman, started by the first one, accepts.
# - Type, Delegate and KillMode as in upstream podman.service.
# - The step's OOM priority, which the runner raises for job processes and podman
#   passes on to containers.
# - StartLimitBurst=1: a podman that dies without accepting would otherwise be
#   re-activated by the still-queued connection, turning a crash into a hang.
#   The interval must be infinity; 0 disables the limit.
# - Output to a file, not the journal, which ingests asynchronously and could
#   still be catching up when fail() reads it.
started_unit="${unit}"
if ! sudo systemd-run --quiet --unit="${unit}" \
    --socket-property=ListenStream="${podman_socket}" \
    --socket-property=SocketUser="${runner_user}" \
    --socket-property=SocketGroup="${runner_group}" \
    --socket-property=SocketMode=0660 \
    --property=Type=exec \
    --property=Delegate=yes \
    --property=KillMode=process \
    --property=OOMScoreAdjust="${oom_score_adj}" \
    --property=StartLimitBurst=1 \
    --property=StartLimitIntervalSec=infinity \
    --property=TimeoutStopSec="${stop_timeout}" \
    --property=StandardOutput=append:"${service_log}" \
    --property=StandardError=append:"${service_log}" \
    "${podman_bin}" system service --time=0; then
  fail "could not start the podman socket unit ${unit}"
fi

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
  22) fail "podman API on ${podman_socket} answered with an HTTP error" ;;
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
