#!/usr/bin/env python3
"""Exercise MCP tools using an owned listener and default dry-run previews only."""

import json
import os
from pathlib import Path
import socket

from stdio_client import run_requests

ROOT = Path(__file__).resolve().parents[2]
BIN = os.environ.get("PORT_DETECTIVE_BIN") or str(
    ROOT / ("port-detective.exe" if os.name == "nt" else "port-detective"))
if not Path(BIN).is_file():
    raise SystemExit("Build the CLI first or set PORT_DETECTIVE_BIN: " + BIN)
os.chdir(ROOT)


def call(request_id, name, arguments):
    return {"jsonrpc": "2.0", "id": request_id, "method": "tools/call",
            "params": {"name": name, "arguments": arguments}}


with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
    listener.bind(("127.0.0.1", 0))
    listener.listen(8)
    port = listener.getsockname()[1]
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as unused:
        unused.bind(("127.0.0.1", 0))
        free_port = unused.getsockname()[1]

    requests = [
        {"jsonrpc": "2.0", "id": 1, "method": "initialize",
         "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                    "clientInfo": {"name": "tool-verifier", "version": "1.0"}}},
        {"jsonrpc": "2.0", "method": "notifications/initialized"},
        call(2, "inspect_port", {"port": port}),
        call(3, "scan_dev_ports", {"ports": f"{port}-{port},{port}"}),
        call(4, "inspect_port", {"port": 70000}),
        call(5, "inspect_port", {"port": free_port}),
        call(6, "clean_current_project", {"project_path": str(ROOT), "ports": str(port)}),
        call(7, "inspect_port", {"port": port + 0.5}),
        call(8, "free_port", {"port": port}),
    ]
    process = run_requests(BIN, requests)
    assert process.returncode == 0, process.stderr.decode("utf-8", "replace")
    lines = [line for line in process.stdout.splitlines() if line.strip()]
    responses = {response["id"]: response
                 for response in map(json.loads, lines)}
    assert len(lines) == 8 and set(responses) == set(range(1, 9)), responses
    assert "result" in responses[1], responses[1]

    def data(request_id):
        result = responses[request_id]["result"]
        assert not result.get("isError", False), result
        return json.loads(result["content"][0]["text"])

    diagnostic = data(2)
    assert diagnostic["is_open"] and diagnostic["pid"] == os.getpid(), diagnostic
    scan = data(3)
    assert len(scan) == 1 and scan[0]["port"] == port, scan
    for invalid in (4, 7):
        assert responses[invalid]["result"].get("isError") is True, responses[invalid]
    assert data(5)["is_open"] is False, responses[5]
    clean = data(6)
    assert clean["dry_run"] is True, clean
    assert os.path.samefile(clean["cwd"], ROOT), clean
    project = diagnostic.get("project") or {}
    identified_project = project.get("path")
    if identified_project and os.path.samefile(identified_project, ROOT):
        assert clean["count"] == 1, clean
    else:
        assert clean["count"] == 0 and clean["cleaned"] == [], clean
        print("SKIP: positive project match; host hides listener working-directory metadata.")
    assert clean["count"] in (0, 1) and len(clean["cleaned"]) == clean["count"], clean
    if clean["cleaned"]:
        assert clean["cleaned"][0]["port"] == port, clean
    preview = data(8)
    assert preview["dry_run"] is True and preview["diagnostic"]["pid"] == os.getpid(), preview
    with socket.create_connection(("127.0.0.1", port), timeout=2):
        pass

print("PASS: MCP inspect, deduplicated range scan, invalid inputs, explicit project,")
print("      safe default cleanup/free previews, listener survival, and JSON-RPC stdout.")
