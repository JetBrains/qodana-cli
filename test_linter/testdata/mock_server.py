#!/usr/bin/env python3
"""Mock server for qodana.cloud API endpoints using Flask."""
# /// script
# dependencies = [
#   "flask>=3.0.0",
# ]
# ///

import gzip
import json
import ssl
import sys
from datetime import datetime
from functools import wraps

from flask import Flask, request, Response

LOG_FILE = "/tmp/mock_server.log"
REQUESTS_FILE = "/tmp/mock_requests.jsonl"
TLS_CERT_FILE = "/tmp/mock_server.crt"
TLS_KEY_FILE = "/tmp/mock_server.key"

MOCK_PROJECT_ID = "test"
MOCK_PRODUCT_CODE = "QDTEST"
MOCK_REPORT_ID = "mock-report-12345"

app = Flask(__name__)
app.url_map.strict_slashes = False


def log(msg):
    """Log message to file with timestamp."""
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    with open(LOG_FILE, "a") as f:
        f.write(f"[{timestamp}] {msg}\n")
        f.flush()


def log_request_entry(entry):
    """Log request/response as JSON line for programmatic access."""
    with open(REQUESTS_FILE, "a") as f:
        f.write(json.dumps(entry) + "\n")
        f.flush()


def is_binary(data):
    """Check if data appears to be binary (non-text) content."""
    if not data:
        return False
    sample = data[:1000] if isinstance(data, bytes) else data[:1000].encode('utf-8', errors='replace')
    non_printable = sum(1 for byte in sample if byte < 32 and byte not in (9, 10, 13))
    return non_printable > len(sample) * 0.1


def parse_json_body(body):
    """Try to parse body as JSON, return None if not valid JSON."""
    if not body or is_binary(body):
        return None
    try:
        if isinstance(body, bytes):
            body = body.decode('utf-8')
        return json.loads(body)
    except (json.JSONDecodeError, TypeError, UnicodeDecodeError):
        return None


def log_endpoint(func):
    """Decorator to log request/response for each endpoint."""
    @wraps(func)
    def wrapper(*args, **kwargs):
        request_body = request.get_data()

        # Log request
        log(f">>> {request.method} {request.path}")
        if request_body:
            if is_binary(request_body):
                log(f"    Request body: <file, {len(request_body)} bytes>")
            else:
                body_preview = request_body.decode('utf-8', errors='replace')[:1000]
                if len(request_body) > 1000:
                    body_preview += "..."
                log(f"    Request body: {body_preview}")

        # Prepare JSON log entry
        request_entry = {
            "timestamp": datetime.now().isoformat(),
            "method": request.method,
            "path": request.path.rstrip('/'),
            "request_body": parse_json_body(request_body) if not is_binary(request_body) else "<binary>",
        }

        # Call the endpoint function
        response = func(*args, **kwargs)

        # Parse response
        body, status = _parse_response(response)

        # Log response
        response_body_entry = body
        if isinstance(body, (bytes, bytearray)):
            response_body_entry = f"<binary, {len(body)} bytes>"

        log(f"<<< {status} (mocked)")
        if body and not isinstance(body, Response):
            body_str = json.dumps(body) if isinstance(body, dict) else str(body)
            log(f"    Response body: {body_str}")

        request_entry["response_status"] = status
        request_entry["response_body"] = response_body_entry
        log_request_entry(request_entry)

        return response

    return wrapper


def _parse_response(response):
    """Extract body and status from Flask response."""
    if isinstance(response, Response):
        return "<Response object>", response.status_code
    if isinstance(response, tuple):
        return response[0], response[1] if len(response) > 1 else 200
    return response, 200


@app.route('/health')
def health():
    """Health check endpoint for Docker health checks."""
    return {"status": "ok"}


@app.route('/api/versions', methods=['GET'])
@log_endpoint
def api_versions():
    """Return API version information."""
    return {
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


@app.route('/api/v1/projects', methods=['GET'])
@log_endpoint
def projects():
    """Return project information."""
    return {
        "id": MOCK_PROJECT_ID,
        "idHash": MOCK_PROJECT_ID,
        "slug": MOCK_PROJECT_ID,
        "name": MOCK_PROJECT_ID,
        "languages": {},
        "organizationId": MOCK_PROJECT_ID,
        "organizationSlug": MOCK_PROJECT_ID,
        "teamId": MOCK_PROJECT_ID,
        "teamSlug": MOCK_PROJECT_ID,
        "isDemo": False,
        "hasNonDefaultBranches": False
    }


@app.route('/api/v1/projects/configuration', methods=['GET'])
@log_endpoint
def projects_configuration():
    """Return project configuration."""
    return {
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


@app.route('/api/v1/s3mock/global/qodana.yaml', methods=['GET'])
@log_endpoint
def s3_global_config():
    """Return gzipped global qodana.yaml config."""
    content = gzip.compress("version: 1.0\nfailThreshold: 0".encode("utf-8"))
    return Response(content, status=200, content_type="application/octet-stream")


@app.route('/linters/v1/linters/license-key', methods=['GET'])
@log_endpoint
def license_key():
    """Return license key information."""
    return {
        "projectIdHash": MOCK_PROJECT_ID,
        "organizationIdHash": MOCK_PROJECT_ID,
        "licenseId": MOCK_PROJECT_ID,
        "licenseKey": MOCK_PROJECT_ID,
        "expirationDate": "2999-06-30",
        "licensePlan": "ULTIMATE_PLUS"
    }


@app.route('/api/v1/reports', methods=['POST'])
@log_endpoint
def create_report():
    """Create a new report and return file upload URLs."""
    return {
        "reportId": MOCK_REPORT_ID,
        "fileLinks": {
            "qodana.sarif.json": "https://mocked.qodana.cloud/api/v1/s3mock/qodana.sarif.json",
            "qodana-short.sarif.json": "https://mocked.qodana.cloud/api/v1/s3mock/qodana-short.sarif.json",
            "log/idea.log": "https://mocked.qodana.cloud/api/v1/s3mock/log/idea.log",
            "donotexist.exe": "https://mocked.qodana.cloud/api/v1/s3mock/donotexist.exe"
        },
        "langsRequired": False
    }


@app.route('/api/v1/s3mock/<path:filepath>', methods=['PUT'])
@log_endpoint
def s3_upload(filepath):
    """Accept file uploads to S3 mock storage."""
    return ""


@app.route('/api/v1/reports/<report_id>/finish', methods=['POST'])
@log_endpoint
def finish_report(report_id):
    """Mark report as finished."""
    return {
        "url": f"https://mocked.qodana.cloud/projects/{MOCK_PROJECT_ID}/reports/{report_id}",
        "token": "mock-share-token-abc123"
    }


@app.route('/fus/v5/send', methods=['POST'])
@log_endpoint
def fus_send():
    """Accept FUS telemetry data."""
    return ""


@app.route('/storage/fus/config/v4/FUS/QDTEST.json', methods=['GET'])
@log_endpoint
def fus_config():
    """Return FUS configuration."""
    return {
        "productCode": MOCK_PRODUCT_CODE,
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


@app.route('/storage/ap/fus/metadata/tiger/FUS/groups/QDTEST.json', methods=['GET'])
@log_endpoint
def fus_groups():
    """Return FUS groups metadata."""
    return {
        "version": "1",
        "groups": [
            {
                "id": "qd.cl.system.os",
                "builds": [{"from": "0", "to": "999999"}],
                "versions": [{"from": "1", "to": "2147483647"}],
                "rules": {
                    "event_id": ["rule:TRUE"],
                    "event_data": {
                        "arch": ["rule:TRUE"],
                        "name": ["rule:TRUE"],
                        "system_qdcld_project_id": ["rule:TRUE"],
                        "version": ["rule:TRUE"]
                    },
                    "enums": {},
                    "regexps": {}
                },
                "anonymized_fields": []
            },
            {
                "id": "qd.cl.lifecycle",
                "builds": [{"from": "0", "to": "999999"}],
                "versions": [{"from": "1", "to": "2147483647"}],
                "rules": {
                    "event_id": ["rule:TRUE"],
                    "event_data": {
                        "system_qdcld_project_id": ["rule:TRUE"],
                        "version": ["rule:TRUE"]
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


@app.route('/storage/ap/fus/metadata/dictionaries', methods=['GET'])
@log_endpoint
def fus_dictionaries():
    """Return FUS dictionaries."""
    return {"dictionaries": []}


def main():
    """Start the HTTPS mock server."""
    log("Starting mock HTTPS server on 127.0.0.1:443")
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(certfile=TLS_CERT_FILE, keyfile=TLS_KEY_FILE)
    log("Mock server ready")
    app.run(host='127.0.0.1', port=443, debug=False, use_reloader=False, ssl_context=context)


if __name__ == "__main__":
    sys.exit(main())
