import json
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import benchmark


class InfrastructureTest(unittest.TestCase):
    def test_mcp_keeps_get_event_stream_open_for_session_lifetime(self):
        attached = threading.Event()
        finished = threading.Event()

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *_):
                pass

            def do_GET(self):
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.end_headers()
                attached.set()
                self.wfile.write(b": heartbeat\n\n")
                self.wfile.flush()
                finished.wait(10)

            def do_POST(self):
                request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                method = request["method"]
                # Emulate eviction once initialization is over unless GET attached.
                status = 404 if method == "tools/list" and not attached.is_set() else 200
                result = {"protocolVersion": "2025-03-26"} if method == "initialize" else {"tools": []}
                body = json.dumps({"id": request.get("id"), "result": result}).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Mcp-Session-Id", "test-session")
                self.end_headers()
                self.wfile.write(body)

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            client = benchmark.McpClient(f"http://127.0.0.1:{server.server_port}/stream")
            try:
                self.assertTrue(attached.wait(1))
                self.assertEqual({"tools": []}, client.request("tools/list"))
                self.assertFalse(client.events_stopped.is_set())
            finally:
                client.close()
                self.assertFalse(client.events_thread.is_alive())
        finally:
            finished.set()
            server.shutdown()
            server.server_close()
            thread.join()

    def test_warm_scans_replace_scripts_and_isolate_ide_configuration(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            project = root / "source"
            project.mkdir()
            (project / "Example.java").write_text("class Example {}")
            workspace = root / "warm"
            commands = []

            class Process:
                returncode = 0

                def wait(self, timeout):
                    return 0

                def poll(self):
                    return 0

            def run(command, **_):
                commands.append(command)
                results = Path(command[command.index("--results-dir") + 1])
                scripts = list((workspace / "project/inspections").glob("*.inspection.kts"))
                self.assertEqual(1, len(scripts))
                rule = scripts[0].name.removesuffix(".inspection.kts")
                benchmark.write_json(results / "qodana.sarif.json", {"runs": [{"tool": {"driver": {
                    "rules": [{"id": rule}]}}, "results": []}]})
                return Process()

            with patch.object(benchmark.subprocess, "Popen", side_effect=run):
                benchmark.scan_project(project, root / "first", {"EdictBenchmarkFirst": "first"}, workspace=workspace)
                (workspace / "cache").mkdir()
                (workspace / "cache/warm-marker").touch()
                benchmark.scan_project(project, root / "second", {"EdictBenchmarkSecond": "second"}, workspace=workspace)
            self.assertTrue((workspace / "cache/warm-marker").exists())
            self.assertTrue(all(f"--property=idea.config.path={workspace / 'cache/config'}" in cmd for cmd in commands))
            mcp = benchmark.qodana_command(project, root / "mcp-results", root / "mcp-cache")
            self.assertIn(f"--property=idea.config.path={root / 'mcp-cache/config'}", mcp)

    def test_transport_failure_stops_further_model_tool_retries(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "log").mkdir()
            class BrokenClient:
                def request(self, *_):
                    raise OSError("session disconnected")
            proxy = benchmark.InspectionProxy(BrokenClient(), [], root, root)
            try:
                with self.assertRaises(OSError):
                    proxy.call("generate_inspection_kts_api", {"language": "Java"})
                with self.assertRaises(benchmark.InfrastructureError):
                    proxy.call("generate_inspection_kts_api", {"language": "Java"})
                self.assertEqual("session disconnected", proxy.fatal_error)
                self.assertEqual(2, len((root / "log/inspection-mcp.jsonl").read_text().splitlines()))
            finally:
                proxy.close()



class PreparationTest(unittest.TestCase):
    def test_seed_preserves_required_evidence_and_holds_out_optional(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            spec = {"ruleId": "BusyWait", "description": "Do not busy wait", "language": "Java",
                    "positiveExamples": [{"path": "X.java"}], "optionalPositiveExamples": [{"path": "Y.java"}]}
            benchmark.write_json(root / "project/benchmark/BusyWait/specification.json", spec)
            with patch.object(benchmark, "git_revision", return_value="a" * 40):
                benchmark.prepare(root / "project", root / "output", 0)
            seeded = json.loads((root / "output/state/clusters/busywait/description.json").read_text())
            self.assertEqual(spec["positiveExamples"], seeded["benchmarkSpecification"]["positiveExamples"])
            self.assertNotIn("optionalPositiveExamples", seeded["benchmarkSpecification"])
            self.assertEqual(spec, json.loads((root / "output/inputs.json").read_text())["specifications"][0])
            self.assertFalse((root / "output/state/clusters/busywait/signals").exists())


if __name__ == "__main__":
    unittest.main()
