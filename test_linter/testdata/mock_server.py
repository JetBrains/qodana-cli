#!/usr/bin/env python3
"""
Mock server for qodana.cloud API endpoints.
Logs full request/response bodies for debugging.
"""

import gzip
import json
import os
import re
import ssl
import subprocess
import threading
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
from datetime import datetime

LOG_FILE = "/tmp/mock_server.log"
REQUESTS_FILE = "/tmp/mock_requests.jsonl"
TLS_CERT_FILE = "/tmp/mock_server.crt"
TLS_KEY_FILE = "/tmp/mock_server.key"
TLS_HOSTS = [
    "resources.jetbrains.com",
    "analytics.services.jetbrains.com",
    "mocked.qodana.cloud",
]

# Mock responses for qodana.cloud endpoints
MOCK_RESPONSES = {
    "GET /api/versions": {
        "status": 200,
        "body": {
          "api": {
            "versions": [
              {
                "version": "1.36",
                "url": "https://mocked.qodana.cloud/api/v1"
              }
            ]
          },
          "linters": {
            "versions": [
              {
                "version": "1.36",
                "url": "https://mocked.qodana.cloud/linters/v1"
              }
            ]
          }
        }
    },
    "GET /api/v1/projects": {
        "status": 200,
        "body": {
            "id": "test",
            "idHash": "test",
            "slug": "test",
            "name": "test",
            "languages": {},
            "organizationId": "test",
            "organizationSlug": "test",
            "teamId": "test",
            "teamSlug": "test",
            "isDemo": False,
            "hasNonDefaultBranches": False
        }
    },
    "GET /api/v1/projects/configuration": {
        "status": 200,
        "body": {
            "id": "main",
            "name": "global configuration",
            "description": "mocked global configuration",
            "files": [
                {
                    "path": "qodana.yaml",
                    "s3Url": "https://mocked.qodana.cloud/api/v1/s3mock/global/qodana.yaml",
                    "vcsUrl": "https://example.com/repo/qodana.yaml",
                    "checksum": "mock-checksum",
                    "fileType": "QODANA_YAML"
                }
            ],
            "createdAt": "2024-01-01T00:00:00Z",
            "lastUpdatedAt": "2024-01-01T00:00:00Z"
        }
    },
    "GET /api/v1/s3mock/global/qodana.yaml": {
        "status": 200,
        "body": gzip.compress("version: 1.0\nfailThreshold: 0".encode("utf-8")),
        "content_type": "application/octet-stream"
    },
    "GET /linters/v1/linters/license-key": {
        "status": 200,
        "body": {
            "projectIdHash": "test",
            "organizationIdHash": "test",
            "licenseId": "test",
            "licenseKey": "test",
            "expirationDate": "2999-06-30",
            "licensePlan": "ULTIMATE_PLUS"
        }
    },
    "POST /api/v1/reports": {
        "status": 200,
        "body": {
            "reportId": "mock-report-12345",
            "fileLinks": {
                "qodana.sarif.json": "https://mocked.qodana.cloud/api/v1/s3mock/qodana.sarif.json",
                "qodana-short.sarif.json": "https://mocked.qodana.cloud/api/v1/s3mock/qodana-short.sarif.json",
                "log/idea.log": "https://mocked.qodana.cloud/api/v1/s3mock/log/idea.log",
                "donotexist.exe": "https://mocked.qodana.cloud/api/v1/s3mock/donotexist.exe"
            },
            "langsRequired": False
        }
    },
    "PUT /api/v1/s3mock": {
        "status": 200,
        "body": ""
    },
    "POST /api/v1/reports/finish": {
        "status": 200,
        "body": {
            "url": "https://mocked.qodana.cloud/projects/test/reports/mock-report-12345",
            "token": "mock-share-token-abc123"
        }
    },
    "POST /fus/v5/send": {
        "status": 200,
        "body": ""
    },
    "GET /storage/fus/config/v4/FUS/QDTEST.json": {
        "status": 200,
        "body": {
          "productCode": "QDTEST",
          "versions": [
            {
              "majorBuildVersionBorders": {
                "from": "2025.1"
              },
              "releaseFilters": [
                {
                  "releaseType": "ALL",
                  "from": 0,
                  "to": 256
                }
              ],
              "endpoints": {
                "send": "https://analytics.services.jetbrains.com/fus/v5/send/",
                "metadata": "https://resources.jetbrains.com/storage/ap/fus/metadata/tiger/FUS/groups/",
                "dictionary": "https://resources.jetbrains.com/storage/ap/fus/metadata/dictionaries/"
              },
              "options": {
                "groupDataThreshold": "10000000",
                "dataThreshold": "10000000",
                "groupAlertThreshold": "6000"
              }
            }
          ]
        }
    },
    "GET /storage/ap/fus/metadata/tiger/FUS/groups/QDTEST.json": {
        "status": 200,
        "body": {
          "version": "1",
          "groups": [
            {
              "id": "qd.cl.system.os",
              "builds": [
                {
                  "from": "0",
                  "to": "999999"
                }
              ],
              "versions": [
                {
                  "from": "1",
                  "to": "2147483647"
                }
              ],
              "rules": {
                "event_id": [
                  "rule:TRUE"
                ],
                "event_data": {
                  "arch": [
                    "rule:TRUE"
                  ],
                  "name": [
                    "rule:TRUE"
                  ],
                  "system_qdcld_project_id": [
                    "rule:TRUE"
                  ],
                  "version": [
                    "rule:TRUE"
                  ]
                },
                "enums": {},
                "regexps": {}
              },
              "anonymized_fields": []
            },
            {
              "id": "qd.cl.lifecycle",
              "builds": [
                {
                  "from": "0",
                  "to": "999999"
                }
              ],
              "versions": [
                {
                  "from": "1",
                  "to": "2147483647"
                }
              ],
              "rules": {
                "event_id": [
                  "rule:TRUE"
                ],
                "event_data": {
                  "system_qdcld_project_id": [
                    "rule:TRUE"
                  ],
                  "version": [
                    "rule:TRUE"
                  ]
                },
                "enums": {},
                "regexps": {}
              },
              "anonymized_fields": []
            }
          ],
          "rules": {
            "event_id": [],
            "event_data": {},
            "enums": {},
            "regexps": {}
          }
        }
    },
    "GET /storage/ap/fus/metadata/dictionaries": {
        "status": 200,
        "body": {
            "dictionaries": []
        }
    }
}

def log(msg):
    """Log message to file with timestamp."""
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    with open(LOG_FILE, "a") as f:
        f.write(f"[{timestamp}] {msg}\n")
        f.flush()


def log_request(entry):
    """Log request/response as JSON line for programmatic access."""
    with open(REQUESTS_FILE, "a") as f:
        f.write(json.dumps(entry) + "\n")
        f.flush()


def is_binary(data):
    """Check if data appears to be binary (non-text) content."""
    if not data:
        return False
    sample = data[:1000] if isinstance(data, str) else data[:1000].decode('utf-8', errors='replace')
    non_printable = sum(1 for c in sample if ord(c) < 32 and c not in '\n\r\t')
    return non_printable > len(sample) * 0.1


def parse_json_body(body):
    """Try to parse body as JSON, return None if not valid JSON."""
    if not body or is_binary(body):
        return None
    try:
        return json.loads(body)
    except (json.JSONDecodeError, TypeError):
        return None

def load_fus_config():
    """Load FUS config JSON from the shared testdata file."""
    try:
        with open(FUS_CONFIG_FILE, "r") as f:
            return json.load(f)
    except Exception as e:
        log(f"Failed to load FUS config from {FUS_CONFIG_FILE}: {e}")
        return None

def load_fus_groups():
    """Load FUS groups JSON from the shared testdata file."""
    try:
        with open(FUS_GROUPS_FILE, "r") as f:
            return json.load(f)
    except Exception as e:
        log(f"Failed to load FUS groups from {FUS_GROUPS_FILE}: {e}")
        return None

def generate_tls_files():
    """Generate a self-signed TLS cert and key using openssl."""
    san = ",".join(f"DNS:{host}" for host in TLS_HOSTS)
    try:
        subprocess.run(
            [
                "openssl",
                "req",
                "-x509",
                "-newkey",
                "rsa:2048",
                "-keyout",
                TLS_KEY_FILE,
                "-out",
                TLS_CERT_FILE,
                "-days",
                "3650",
                "-nodes",
                "-subj",
                f"/CN={TLS_HOSTS[0]}",
                "-addext",
                f"subjectAltName={san}",
            ],
            check=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
    except FileNotFoundError:
        log("openssl not found; unable to generate TLS cert/key")
        return False
    except subprocess.CalledProcessError as e:
        log(f"openssl failed: {e.stderr.strip()}")
        return False
    os.chmod(TLS_KEY_FILE, 0o600)
    return True

def ensure_tls_files():
    """Ensure TLS cert and key exist for HTTPS server."""
    if not os.path.exists(TLS_CERT_FILE) or not os.path.exists(TLS_KEY_FILE):
        log("TLS cert/key missing; generating self-signed certificate")
        if not generate_tls_files():
            sys.exit(1)
    if not os.path.exists(TLS_CERT_FILE):
        log(f"Missing TLS cert: {TLS_CERT_FILE}")
        sys.exit(1)
    if not os.path.exists(TLS_KEY_FILE):
        log(f"Missing TLS key: {TLS_KEY_FILE}")
        sys.exit(1)
    os.chmod(TLS_KEY_FILE, 0o600)

class MockHandler(BaseHTTPRequestHandler):
    """HTTP request handler that mocks qodana.cloud endpoints."""

    def log_message(self, format, *args):
        """Override to suppress default logging."""
        pass

    def _read_body(self):
        """Read request body if present."""
        content_length = self.headers.get('Content-Length')
        if content_length:
            return self.rfile.read(int(content_length)).decode('utf-8', errors='replace')
        return None

    def _send_response(self, status, body, content_type="application/json", extra_headers=None):
        """Send HTTP response."""
        if isinstance(body, dict):
            body = json.dumps(body)
        body_bytes = body.encode('utf-8') if isinstance(body, str) else body

        self.send_response(status)
        self.send_header('Content-Type', content_type)
        self.send_header('Content-Length', len(body_bytes))
        if extra_headers:
            for header, value in extra_headers.items():
                self.send_header(header, value)
        self.end_headers()
        self.wfile.write(body_bytes)

    def _handle_request(self, method):
        """Handle incoming request."""
        path = self.path.rstrip('/')
        request_body = self._read_body()

        # Log request (human readable)
        log(f">>> {method} {self.path}")
        if request_body:
            if is_binary(request_body):
                log(f"    Request body: <file, {len(request_body)} bytes>")
            else:
                body_preview = request_body[:1000] + "..." if len(request_body) > 1000 else request_body
                log(f"    Request body: {body_preview}")

        # Prepare JSON log entry
        request_entry = {
            "timestamp": datetime.now().isoformat(),
            "method": method,
            "path": path,
            "request_body": parse_json_body(request_body) if not is_binary(request_body) else "<binary>",
        }

        # Find matching mock response
        response = None

        # Exact match
        key = f"{method} {path}"
        if not response and key in MOCK_RESPONSES:
            response = MOCK_RESPONSES[key]

        # Pattern matches
        if not response:
            if method == "PUT" and path.startswith("/api/v1/s3mock"):
                response = MOCK_RESPONSES["PUT /api/v1/s3mock"]
            elif method == "POST" and re.match(r"^/api/v1/reports/[^/]+/finish$", path):
                response = MOCK_RESPONSES["POST /api/v1/reports/finish"]
            elif method == "POST" and re.match(r"^/api/v1/reports$", path):
                response = MOCK_RESPONSES["POST /api/v1/reports"]

        if response:
            status = response["status"]
            body = response["body"]
            content_type = response.get("content_type")
            if content_type is None:
                content_type = "text/plain" if isinstance(body, str) and not body else "application/json"
            extra_headers = response.get("headers")
            response_body_entry = body
            if isinstance(body, (bytes, bytearray)):
                response_body_entry = f"<binary, {len(body)} bytes>"

            log(f"<<< {status} (mocked)")
            if body:
                body_str = json.dumps(body) if isinstance(body, dict) else str(body)
                log(f"    Response body: {body_str}")

            request_entry["response_status"] = status
            request_entry["response_body"] = response_body_entry
            log_request(request_entry)
            self._send_response(status, body, content_type, extra_headers)
        else:
            log(f"<<< 404 (no mock found)")
            request_entry["response_status"] = 404
            request_entry["response_body"] = {"error": "Not found", "path": self.path}
            log_request(request_entry)
            self._send_response(404, {"error": "Not found", "path": self.path})

    def do_GET(self):
        self._handle_request("GET")

    def do_POST(self):
        self._handle_request("POST")

    def do_PUT(self):
        self._handle_request("PUT")

    def do_DELETE(self):
        self._handle_request("DELETE")


def main():
    """Start the mock server."""
    host = "0.0.0.0"
    http_port = 80
    https_port = 443

    ensure_tls_files()

    log(f"Starting mock HTTP server on {host}:{http_port}")

    http_server = HTTPServer((host, http_port), MockHandler)
    https_server = HTTPServer((host, https_port), MockHandler)

    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(certfile=TLS_CERT_FILE, keyfile=TLS_KEY_FILE)
    https_server.socket = context.wrap_socket(https_server.socket, server_side=True)

    threading.Thread(target=https_server.serve_forever, daemon=True).start()

    log(f"Starting mock HTTPS server on {host}:{https_port}")
    log("Mock server ready")
    http_server.serve_forever()


if __name__ == "__main__":
    main()
