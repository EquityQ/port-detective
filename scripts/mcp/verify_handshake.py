#!/usr/bin/env python3
"""End-to-end acceptance checks for the MCP Stdio protocol.

Communicate with `port-detective mcp` through a real subprocess pipe and verify:
  1. The initialize handshake succeeds.
  2. tools/list returns the expected tools.
  3. Every stdout line is valid JSON-RPC, with no log output mixed in.
"""
import json
import os
import sys

from stdio_client import run_requests

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
    {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "step4-verifier", "version": "1.0"},
        },
    },
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}},
]

proc = run_requests(BIN, REQUESTS)

out_lines = [l for l in proc.stdout.decode("utf-8", "replace").splitlines() if l.strip()]
err_text = proc.stderr.decode("utf-8", "replace")

print(f"exit code      : {proc.returncode}")
print(f"stdout lines   : {len(out_lines)}")
print(f"stderr lines   : {len(err_text.splitlines())}")

# ---- Assertion 1: every stdout line is valid JSON ----
bad = []
responses = []
for line in out_lines:
    try:
        responses.append(json.loads(line))
    except json.JSONDecodeError as e:
        bad.append((line[:120], str(e)))

print("\n--- stdout protocol check ---")
if bad:
    print(f"  FAIL: {len(bad)} lines are not valid JSON (stdout is polluted)")
    for line, e in bad:
        print(f"    {line!r} -> {e}")
else:
    print(f"  PASS: all {len(out_lines)} lines are valid JSON-RPC")

# ---- Assertion 2: initialize returns server information ----
init = next((r for r in responses if r.get("id") == 1), None)
print("\n--- initialize ---")
if init and "result" in init:
    si = init["result"].get("serverInfo", {})
    print(f"  PASS: serverInfo = {si}")
else:
    print(f"  FAIL: {init}")

# ---- Assertion 3: tools/list returns the expected tools ----
tools_resp = next((r for r in responses if r.get("id") == 2), None)
print("\n--- tools/list ---")
expected = {"inspect_port", "scan_dev_ports", "free_port", "clean_current_project"}
if tools_resp and "result" in tools_resp:
    tools = tools_resp["result"].get("tools", [])
    names = {t["name"] for t in tools}
    print(f"  tool count: {len(tools)}")
    for t in tools:
        ann = t.get("annotations", {})
        print(
            f"    - {t['name']:<16} title={ann.get('title')!r} "
            f"readOnly={ann.get('readOnlyHint')} destructive={ann.get('destructiveHint')}"
        )
    print(f"  expected tools present: {expected == names}")
    if expected != names:
        print(f"    missing {expected - names}, extra {names - expected}")
else:
    print(f"  FAIL: {tools_resp}")

# ---- Assertion 4: startup messages on stderr do not affect the protocol ----
print("\n--- stderr (allowed diagnostics) ---")
for line in err_text.splitlines()[:4]:
    print(f"  {line}")

ok = proc.returncode == 0 and (not bad) and init and "result" in init and tools_resp and expected == {t["name"] for t in (tools_resp.get("result", {}).get("tools", []))}
if ok:
    selected = next(t for t in tools_resp["result"]["tools"] if t["name"] == "clean_current_project")
    properties = selected["inputSchema"]["properties"]
    ok = {"project_path", "ports", "dry_run"} <= properties.keys() and properties["dry_run"].get("default") is True
print(f"\n=== Result: {'PASS' if ok else 'FAIL'} ===")
sys.exit(0 if ok else 1)
