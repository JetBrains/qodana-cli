#!/usr/bin/env python3
"""Jenkins generation benchmark using the published Kotlin managed skills.

The host owns fixtures, state and MCP lifecycle. Kotlin owns comparison. Model execution uses
CodexRunner from edict-cli.jar; agents never own authoritative state files.
"""
import argparse
import contextlib
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import shutil
import signal
import socket
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.request import Request, urlopen
from urllib.parse import urlsplit


TOOLS = {"generate_psi_tree", "generate_inspection_kts_api",
         "generate_inspection_kts_examples", "run_inspection_kts"}
QODANA = "/opt/idea/bin/qodana"


class InfrastructureError(RuntimeError):
    """A broken runner must stop generation, not become negative training evidence."""


PROBE_CODE = '''
val probe = localInspection { file, inspection ->
    if (file.text.contains("EDICT_PROBE_MARKER")) {
        inspection.registerProblem(file, "Benchmark infrastructure probe")
    }
}
listOf(InspectionKts(id = "EdictBenchmarkInfrastructureProbe", localTool = probe,
    name = "Benchmark infrastructure probe", htmlDescription = "<html>Probe</html>",
    level = HighlightDisplayLevel.WARNING))
'''


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2) + "\n")


def tc(kind, **attrs):
    def escape(value):
        return str(value).replace("|", "||").replace("'", "|'").replace("\n", "|n").replace("\r", "|r").replace("[", "|[").replace("]", "|]")
    print("##teamcity[" + kind + " " + " ".join(f"{k}='{escape(v)}'" for k, v in attrs.items()) + "]", flush=True)


class McpClient:
    def __init__(self, url):
        self.url = url
        self.session = None
        self.counter = 0
        self.lock = threading.Lock()
        result = self.request("initialize", {"protocolVersion": "2025-03-26", "capabilities": {},
                                            "clientInfo": {"name": "edict-benchmark", "version": "1"}})
        self.protocol = result.get("protocolVersion", "2025-03-26")
        self.request("notifications/initialized", notification=True)
        # IntelliJ evicts pending sessions after 15 seconds unless GET /stream
        # stays open. A successful tools/list alone does not test this lifecycle.
        endpoint = urlsplit(url)
        connection = http.client.HTTPSConnection if endpoint.scheme == "https" else http.client.HTTPConnection
        self.events_connection = connection(endpoint.hostname, endpoint.port, timeout=30)
        self.events_connection.request("GET", endpoint.path + ("?" + endpoint.query if endpoint.query else ""),
                                       headers={"Accept": "text/event-stream", "Mcp-Session-Id": self.session,
                                                "MCP-Protocol-Version": self.protocol})
        self.events_socket = self.events_connection.sock
        self.events = self.events_connection.getresponse()
        if self.events.status != 200:
            self.events.close()
            self.events_connection.close()
            raise InfrastructureError(f"MCP event stream returned HTTP {self.events.status}")
        self.events_stopped = threading.Event()
        self.events_thread = threading.Thread(target=self.drain_events, daemon=True)
        self.events_thread.start()

    def drain_events(self):
        try:
            while self.events.readline():
                pass
        except (OSError, ValueError, http.client.HTTPException):
            pass
        finally:
            self.events_stopped.set()

    def close(self):
        if self.events_socket:
            with contextlib.suppress(OSError):
                self.events_socket.shutdown(socket.SHUT_RDWR)
        self.events_connection.close()
        self.events_thread.join(timeout=5)
        self.events.close()

    def request(self, method, params=None, notification=False):
        with self.lock:
            if hasattr(self, "events_stopped") and self.events_stopped.is_set():
                raise InfrastructureError("Inspections MCP event stream disconnected")
            self.counter += 1
            body = {"jsonrpc": "2.0", "method": method, "params": params or {}}
            if not notification:
                body["id"] = self.counter
            headers = {"Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
            if self.session:
                headers["Mcp-Session-Id"] = self.session
            if hasattr(self, "protocol"):
                headers["MCP-Protocol-Version"] = self.protocol
            with urlopen(Request(self.url, json.dumps(body).encode(), headers), timeout=600) as response:
                self.session = response.headers.get("Mcp-Session-Id", self.session)
                if notification or response.status == 202:
                    return {}
                if "text/event-stream" in response.headers.get("Content-Type", ""):
                    data = []
                    for raw in response:
                        line = raw.decode().rstrip("\r\n")
                        if line.startswith("data:"):
                            data.append(line[5:].lstrip())
                        elif not line and data:
                            result = json.loads("\n".join(data))
                            data.clear()
                            if result.get("id") == body["id"]:
                                break
                    else:
                        raise RuntimeError("MCP event stream ended without a response")
                else:
                    result = json.load(response)
            if "error" in result:
                raise RuntimeError(f"MCP {method}: {result['error']}")
            return result["result"]


def runtime_command(command, pid_file):
    """The host has Python; the unchanged image provides Java, Codex and Qodana."""
    container = os.environ.get("BENCHMARK_CONTAINER")
    if not container:
        return command
    return ["docker", "exec", container, "setsid", "--wait", "/bin/bash", "-c",
            'mkdir -p "$(dirname "$1")"; echo $$ > "$1"; shift; exec "$@"',
            "benchmark", str(pid_file), *command]


def stop(process, pid_file=None):
    if process is None or process.poll() is not None:
        return
    container = os.environ.get("BENCHMARK_CONTAINER")
    if container and pid_file and pid_file.exists():
        pid = int(pid_file.read_text().strip())
        if pid > 1:
            subprocess.run(["docker", "exec", container, "/bin/kill", "-TERM", "--", f"-{pid}"],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10, check=False)
    if process.poll() is not None:
        return
    os.killpg(process.pid, signal.SIGTERM)
    try:
        process.wait(timeout=20)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait()


def qodana_command(project, results, cache, *extra):
    return runtime_command([QODANA, "scan", "--project-dir", str(project), "--results-dir", str(results),
            "--cache-dir", str(cache), "--disable-sanity", "--run-promo=false", "--save-report=false",
            "--property=idea.headless.enable.statistics=false",
            f"--property=idea.config.path={cache / 'config'}", *extra], cache / "runner.pid")


@contextlib.contextmanager
def inspection_server(project, output):
    log = output / "log/inspection-server.log"
    log.parent.mkdir(parents=True, exist_ok=True)
    process = None
    client = None
    try:
        with log.open("w") as stream:
            process = subprocess.Popen(qodana_command(project, output / "mcp-results", output / "mcp-cache",
                                                     "--script", "mcp-server", "--profile-name", "empty"),
                                       stdout=stream, stderr=subprocess.STDOUT, start_new_session=True)
            deadline = time.monotonic() + 1200
            progress_at = time.monotonic()
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise RuntimeError(f"Inspections MCP exited {process.returncode}; see {log}")
                match = re.search(r"Streamable HTTP endpoint:\s*(https?://[^\s\x1b]+)", log.read_text(errors="replace"))
                if match:
                    client = McpClient(match[1])
                    tools = client.request("tools/list")["tools"]
                    missing = TOOLS - {t["name"] for t in tools}
                    if missing:
                        raise RuntimeError(f"Inspections MCP is missing required tools: {sorted(missing)}")
                    write_json(output / "inspection-tools.json", tools)
                    tc("message", text="Inspections MCP ready: all four compiler/documentation tools verified")
                    yield client, tools
                    return
                if time.monotonic() >= progress_at:
                    tc("message", text="Waiting for the inspections MCP project import and HTTP endpoint; see inspection-server.log")
                    progress_at = time.monotonic() + 30
                time.sleep(1)
            raise TimeoutError(f"Inspections MCP did not become ready in 20 minutes; see {log}")
    finally:
        if client:
            client.close()
        stop(process, output / "mcp-cache/runner.pid")


def copy_source(project, destination):
    # Keep generated scripts and import output away from the source evidence.
    shutil.copytree(project, destination, ignore=shutil.ignore_patterns(
        ".git", ".edict", ".qodana", "benchmark", "inspections", "target", "qodana.yaml"))


def scan_project(project, output, codes, timeout=1800, workspace=None):
    """Run only the supplied custom inspections, on the entire inspected project."""
    output.mkdir(parents=True, exist_ok=True)
    workspace = workspace or output
    inspected = workspace / "project"
    if not inspected.exists():
        copy_source(project, inspected)
    shutil.rmtree(inspected / "inspections", ignore_errors=True)
    for rule, code in codes.items():
        destination = inspected / "inspections" / f"{rule}.inspection.kts"
        destination.parent.mkdir(exist_ok=True)
        destination.write_text(code)
    write_json(inspected / "qodana.yaml", {"version": "1.0", "profile": {"name": "empty"},
                                           "include": [{"name": rule} for rule in codes]})
    results = output / "results"
    process = None
    try:
        with (output / "analysis.log").open("w") as log:
            process = subprocess.Popen(qodana_command(inspected, results, workspace / "cache"),
                                       stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            status = process.wait(timeout=timeout)
        if status != 0:
            raise InfrastructureError(f"Project analysis exited {status}; see {output / 'analysis.log'}")
        sarif_path = results / "qodana.sarif.json"
        sarif = json.loads(sarif_path.read_text())
        runs = sarif.get("runs", [])
        if not runs:
            raise RuntimeError("Project analysis returned no SARIF run")
        registered = {r["id"] for run in runs for component in [run.get("tool", {}).get("driver", {})]
                      + run.get("tool", {}).get("extensions", []) for r in component.get("rules", [])}
        if not set(codes) <= registered:
            raise RuntimeError(f"Generated inspections were not loaded: {sorted(set(codes) - registered)}")
        return sarif
    finally:
        stop(process, workspace / "cache/runner.pid")
        if workspace == output:
            shutil.rmtree(inspected, ignore_errors=True)
            shutil.rmtree(output / "cache", ignore_errors=True)


class InspectionProxy:
    """Expose generic tools only; full-project execution also uses disposable scratch."""
    def __init__(self, client, tools, project, output):
        self.client, self.project, self.output = client, project, output
        self.audit_lock = threading.Lock()
        self.scan_lock = threading.Lock()
        self.fatal_error = None
        self.tools = [t for t in tools if t["name"] in TOOLS]
        self.tools.append({"name": "run_project_inspection", "description":
                          "Compile and execute exactly one inspection on the entire source project. "
                          "Returns complete SARIF findings and candidate hash. Does not write Edict state.",
                          "inputSchema": {"type": "object", "properties": {
                              "inspectionKtsCode": {"type": "string"}, "inspectionId": {"type": "string"}},
                              "required": ["inspectionKtsCode", "inspectionId"], "additionalProperties": False}})
        proxy = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                request = {}
                try:
                    length = int(self.headers.get("Content-Length", "0"))
                    if not 0 < length <= 20 * 1024 * 1024:
                        self.send_error(413)
                        return
                    request = json.loads(self.rfile.read(length))
                    if "id" not in request:
                        self.send_response(202)
                        self.end_headers()
                        return
                    method, params = request["method"], request.get("params", {})
                    if method == "initialize":
                        result = {"protocolVersion": params["protocolVersion"], "capabilities": {"tools": {}},
                                  "serverInfo": {"name": "inspection", "version": "1"}}
                    elif method == "tools/list":
                        result = {"tools": proxy.tools}
                    elif method == "ping":
                        result = {}
                    elif method == "tools/call":
                        result = proxy.call(params["name"], params.get("arguments", {}))
                    else:
                        raise ValueError(f"Unsupported method: {method}")
                    response = {"jsonrpc": "2.0", "id": request["id"], "result": result}
                except Exception as error:
                    response = {"jsonrpc": "2.0", "id": request.get("id"),
                                "error": {"code": -32603, "message": str(error)}}
                body = json.dumps(response).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.url = f"http://127.0.0.1:{self.server.server_port}/mcp"

    def call(self, name, arguments):
        started = time.monotonic()
        try:
            if self.fatal_error:
                raise InfrastructureError(self.fatal_error)
            result = self.execute(name, arguments)
        except Exception as error:
            self.audit(name, started, error=str(error))
            if isinstance(error, (InfrastructureError, OSError, subprocess.TimeoutExpired)):
                self.fatal_error = str(error)
            raise
        self.audit(name, started, arguments=arguments, result=result)
        return result

    def audit(self, name, started, **details):
        with self.audit_lock:
            with (self.output / "log/inspection-mcp.jsonl").open("a") as log:
                log.write(json.dumps({"tool": name, "elapsedSeconds": round(time.monotonic() - started, 3),
                                      **details}) + "\n")

    def execute(self, name, arguments):
        if name == "run_project_inspection":
            code, rule = arguments["inspectionKtsCode"], arguments["inspectionId"]
            if not re.fullmatch(r"EdictBenchmark[A-Za-z0-9]+", rule):
                raise ValueError("Use the assigned EdictBenchmark<OriginalRuleId> inspection ID")
            digest = hashlib.sha256(code.encode()).hexdigest()
            destination = self.output / "project-runs" / digest
            with self.scan_lock:
                cached = destination / "response.json"
                if cached.exists():
                    result = json.loads(cached.read_text())
                else:
                    if destination.exists():
                        destination.rename(destination.with_name(f"{digest}-failed-{time.time_ns()}"))
                    destination.mkdir(parents=True, exist_ok=False)
                    sarif = scan_project(self.project, destination, {rule: code},
                                         workspace=self.output / "scan-workspace")
                    result = {"candidateHash": digest, "sarif": sarif,
                              "sourceRevision": git_revision(self.project)}
                    write_json(cached, result)
            result = {"content": [{"type": "text", "text": json.dumps(result)}], "isError": False}
        elif name in TOOLS:
            arguments = dict(arguments, projectPath=str(self.project))
            result = self.client.request("tools/call", {"name": name, "arguments": arguments})
        else:
            raise ValueError("Only generic inspection tools are exposed")
        return result

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


def check_execution(proxy):
    """Check real execution while both IDE processes coexist, before spending model time."""
    tc("message", text="Checking MCP compiler execution and simultaneous full-project scanning")
    started = time.monotonic()
    result = proxy.call("run_inspection_kts", {
        "inspectionKtsCode": PROBE_CODE, "contextPath": "core/src/main/java/BenchmarkProbe.java",
        "targetFileContent": "class BenchmarkProbe { /* EDICT_PROBE_MARKER */ }"})
    structured = result.get("structuredContent")
    if structured is None:
        for item in result.get("content", []):
            try:
                structured = json.loads(item.get("text", ""))
            except (ValueError, TypeError):
                continue
            if isinstance(structured, dict) and "compilationSuccess" in structured:
                break
    if result.get("isError") or not isinstance(structured, dict) or not structured.get("compilationSuccess") or not structured.get("foundProblems"):
        write_json(proxy.output / "log/compiler-probe.json", result)
        raise InfrastructureError("MCP compiler/execution probe failed; see log/compiler-probe.json")
    proxy.call("run_project_inspection", {"inspectionKtsCode": PROBE_CODE,
                                          "inspectionId": "EdictBenchmarkInfrastructureProbe"})
    # Explicitly cross IntelliJ's 15-second pending-session eviction deadline.
    remaining = 16 - (time.monotonic() - started)
    if remaining > 0:
        time.sleep(remaining)
    docs = proxy.call("generate_inspection_kts_api", {"language": "Java"})
    if docs.get("isError"):
        raise InfrastructureError("MCP session check failed after project scan")
    tc("message", text=f"Execution preflight passed in {time.monotonic() - started:.0f}s: "
                       "MCP compiled/reported a fixture, project scan loaded the script, MCP session remained live")


def git_revision(project):
    return subprocess.check_output(["git", "-C", str(project), "rev-parse", "HEAD"], text=True).strip()


def prepare(project, output, limit, rules=""):
    specs = [json.loads(p.read_text()) for p in sorted((project / "benchmark").glob("*/specification.json"))]
    if rules:
        selected = set(rules.split(","))
        missing = selected - {spec["ruleId"] for spec in specs}
        if missing:
            raise ValueError(f"Unknown benchmark rules: {sorted(missing)}")
        specs = [spec for spec in specs if spec["ruleId"] in selected]
    if limit:
        specs = specs[:limit]
    if not specs:
        raise ValueError("No benchmark specifications found")
    revision = git_revision(project)
    mapping = {}
    for spec in specs:
        rule = spec["ruleId"]
        if not re.fullmatch(r"[A-Za-z0-9]+", rule):
            raise ValueError(f"Unsupported rule ID: {rule}")
        cluster = rule.lower()
        mapping[cluster] = rule
        root = output / "state/clusters" / cluster
        generation_spec = {key: value for key, value in spec.items() if not key.startswith("optional")}
        write_json(root / "description.json", {"id": cluster, "description": spec["description"],
                   "language": spec["language"], "status": "Pending", "benchmarkSpecification": generation_spec})
        (root / "history.md").write_text(
            f"# Benchmark input\n\nImported unchanged from benchmark/{rule}/specification.json at {revision}.\n"
            "These are labelled benchmark examples, not correcting commits or PRs. "
            "No commit/PR Signals have been fabricated. The original required examples are the source evidence; "
            "optional examples are reserved for independent scoring.\n")
    write_json(output / "inputs.json", {"revision": revision, "clusterToRule": mapping, "specifications": specs})
    # The normal cluster skill can consume transient evidence through edict-code-example.
    # This adapter explicitly describes the benchmark's source contract instead of inventing valid-looking Git signals.
    prompt = f"""$edict_manager
Run the Kotlin managed generation workflow for these Pending benchmark clusters: {', '.join(mapping)}.
Source project: {project}. State is managed exclusively through edict-mcp. Scratch: {output / 'scratch'}.
Each cluster's description.benchmarkSpecification contains the original required examples and description.
This is a generation-only benchmark: do not extract commits, distribute inbox entries, rename, merge or split clusters.
Delegate edict-generation, which delegates edict-cluster-generation normally. Process clusters sequentially so that
the five-level generation/review hierarchy fits the six-frame capacity. Close completed native agents.

Benchmark evidence adaptation (pass these instructions to every generation worker):
There are no persisted commit/PR signals. Required positiveExamples and negativeExamples are labelled source evidence
at Git revision {revision}, unless an example explicitly gives another revision. For each required example create a
transient scratch signal containing its actual fileRevision/path/revision/expectedRanges, label, original description,
and source.type SubmittedFeedback with the originating specification path. Delegate edict-code-example with example.write
on this cluster's synthetic-examples directory, and no cluster.signal.write, to create a small source-faithful example.
Use its returned example ID in subsequent validation. Do not persist these transient feedback records as commit/PR signals.
When required labels conflict at identical source locations, record the exact conflicting evidence and a valid domain
outcome; do not silently relabel or remove fixtures. Optional examples and gold SARIF are scoring data: do not use them
to guide generation. Follow all ordinary compilation, code review, weak-signal review and value-review requirements.
Use the inspection tool run_project_inspection for complete project findings, with the exact stored candidate bytes.
Use exactly one InspectionKts descriptor with id EdictBenchmark<OriginalRuleId> (for example EdictBenchmarkBusyWait).
This avoids executing the IDE's built-in inspection with the same original ID during scoring.
Do not call legacy Edict inspection-server tools. Preserve original specification metadata.
Finish all managed tasks and report each cluster's persisted status and accepted inspection path.
"""
    (output / "prompt.txt").write_text(prompt)
    return specs, mapping


def export_analysis(project, output, mapping):
    """Produce generation artifacts only; the separate Kotlin Gradle task owns comparison."""
    codes = {}
    for cluster, rule in mapping.items():
        description = json.loads((output / "state/clusters" / cluster / "description.json").read_text())
        if description["status"] == "Generated":
            codes[f"EdictBenchmark{rule}"] = (output / "state/inspections" / f"{cluster}.inspection.kts").read_text()
    sarif = scan_project(project, output / "evaluation", codes, workspace=output / "scan-workspace") if codes else {"runs": [{"results": []}]}
    write_json(output / "qodana.sarif.json", sarif)
    tc("message", text=f"Generation artifacts ready: {len(codes)}/{len(mapping)} inspections; Kotlin comparison runs next")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for option in ("project", "output", "jar", "model"):
        parser.add_argument("--" + option, required=True)
    parser.add_argument("--minutes", type=int, default=240)
    parser.add_argument("--limit", type=int, default=0)
    parser.add_argument("--rules", default="")
    parser.add_argument("--preflight", choices=("true", "false"), default="false")
    args = parser.parse_args()
    project, output = Path(args.project).resolve(), Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    if (output / "state").exists():
        raise ValueError("Use a fresh output directory for each benchmark run")
    specs, mapping = prepare(project, output, args.limit, args.rules)
    generation_error = None
    with inspection_server(project, output) as (client, tools):
        proxy = InspectionProxy(client, tools, project, output)
        try:
            check_execution(proxy)
            command = runtime_command(["/bin/bash", str(Path(__file__).with_name("run.sh")),
                                       str(project), str(output), args.jar, proxy.url, args.model,
                                       str(args.minutes), args.preflight], output / "host.pid")
            with subprocess.Popen(command, start_new_session=True) as host:
                agent_log = output / "log/edict/edict-agent-short.log"
                offset = 0
                while True:
                    if proxy.fatal_error:
                        stop(host, output / "host.pid")
                        generation_error = InfrastructureError(proxy.fatal_error)
                    finished = host.poll() is not None
                    if agent_log.exists():
                        with agent_log.open() as log:
                            log.seek(offset)
                            # Kotlin AgentLogger has already redacted all issued capability tokens.
                            for line in log:
                                tc("message", text=line.rstrip())
                            offset = log.tell()
                    if finished:
                        if host.returncode:
                            generation_error = generation_error or subprocess.CalledProcessError(host.returncode, command)
                        break
                    time.sleep(1)
        finally:
            proxy.close()
            write_json(output / "progress.json", {
                "generationError": str(generation_error) if generation_error else proxy.fatal_error,
                "clusters": {rule: json.loads((output / "state/clusters" / cluster / "description.json").read_text())["status"]
                             for cluster, rule in mapping.items()}})
    if args.preflight == "true":
        tc("buildStatus", text="PREFLIGHT: inspections MCP and Kotlin managed skills ready; sandbox verified")
    else:
        export_analysis(project, output, mapping)
    if generation_error:
        raise generation_error


if __name__ == "__main__":
    main()
