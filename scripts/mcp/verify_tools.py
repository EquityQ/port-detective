#!/usr/bin/env python3
"""End-to-end read-only checks for MCP tools/call.

Safety: this script calls only read-only tools (inspect_port and scan_dev_ports).
Never call free_port; validate destructive tools only with controlled temporary ports.
"""
import json
import os
import subprocess
import sys

# Binary resolution order: PORT_DETECTIVE_BIN, then the repository build artifact.
_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_BIN = os.environ.get("PORT_DETECTIVE_BIN") or os.path.join(
    _ROOT, "port-detective.exe" if os.name == "nt" else "port-detective")
if not os.path.exists(_BIN):
    sys.stderr.write(
        "Binary not found: %s\nRun `go build -o port-detective .` first, "
        "or set PORT_DETECTIVE_BIN.\n" % _BIN)
    sys.exit(2)
BIN = _BIN

REQUESTS = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                "clientInfo": {"name": "verifier", "version": "1.0"}}},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "tools/call",
     "params": {"name": "inspect_port", "arguments": {"port": 1}}},
    {"jsonrpc": "2.0", "id": 3, "method": "tools/call",
     "params": {"name": "scan_dev_ports", "arguments": {}}},
    {"jsonrpc": "2.0", "id": 4, "method": "tools/call",
     "params": {"name": "inspect_port", "arguments": {"port": 70000}}},
    {"jsonrpc": "2.0", "id": 5, "method": "tools/call",
     "params": {"name": "inspect_port", "arguments": {"port": 8080}}},
]

payload = "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in REQUESTS)
proc = subprocess.run([BIN, "mcp"], input=payload.encode("utf-8"),
                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)

responses, bad = {}, []
for line in proc.stdout.decode("utf-8", "replace").splitlines():
    if not line.strip():
        continue
    try:
        obj = json.loads(line)
        responses[obj.get("id")] = obj
    except json.JSONDecodeError as e:
        bad.append((line[:100], str(e)))


def tool_text(resp_id):
    r = responses.get(resp_id, {}).get("result", {})
    content = r.get("content") or [{}]
    return r.get("isError"), content[0].get("text", "<empty>")


out = []


def show(label, resp_id, limit=600):
    is_err, text = tool_text(resp_id)
    out.append(f"\n=== {label} ===")
    out.append(f"  isError={is_err}")
    out.append("  " + (text if len(text) <= limit else text[:limit] + " ...(truncated)"))


show("inspect_port(1): controlled system port probe", 2)
show("scan_dev_ports(): default development port set", 3)
show("inspect_port(70000): invalid port, isError should be true", 4)
show("inspect_port(8080): idle port, is_open should be false", 5)

out.append("\n=== stdout protocol check ===")
out.append(f"  invalid JSON lines = {len(bad)} -> {'PASS' if not bad else 'FAIL ' + str(bad)}")
out.append(f"  responses received = {len(responses)} (expected 5)")

err4 = tool_text(4)[0]
err_lines = [l for l in proc.stderr.decode("utf-8", "replace").splitlines() if l.strip()]
out.append(f"\n=== stderr (allowed diagnostics, {len(err_lines)} lines) ===")
out.append("  " + (err_lines[0] if err_lines else "<empty>"))

sys.stdout.buffer.write("\n".join(out).encode("utf-8"))
sys.stdout.buffer.flush()

ok = (not bad) and len(responses) == 5 and err4 is True
sys.exit(0 if ok else 1)
