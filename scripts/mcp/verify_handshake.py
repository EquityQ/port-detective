#!/usr/bin/env python3
"""End-to-end acceptance checks for the MCP Stdio protocol.

Communicate with `port-detective mcp` through a real subprocess pipe and verify:
  1. The initialize handshake succeeds.
  2. tools/list returns the expected tools.
  3. Every stdout line is valid JSON-RPC, with no log output mixed in.
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
        "找不到二进制 %s\n请先执行 `go build -o port-detective .`，"
        "或用环境变量 PORT_DETECTIVE_BIN 指定路径。\n" % _BIN)
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

payload = "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in REQUESTS)

proc = subprocess.run(
    [BIN, "mcp"],
    input=payload.encode("utf-8"),
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE,
    timeout=60,
)

out_lines = [l for l in proc.stdout.decode("utf-8", "replace").splitlines() if l.strip()]
err_text = proc.stderr.decode("utf-8", "replace")

print(f"exit code      : {proc.returncode}")
print(f"stdout 行数    : {len(out_lines)}")
print(f"stderr 行数    : {len(err_text.splitlines())}")

# ---- Assertion 1: every stdout line is valid JSON ----
bad = []
responses = []
for line in out_lines:
    try:
        responses.append(json.loads(line))
    except json.JSONDecodeError as e:
        bad.append((line[:120], str(e)))

print("\n--- stdout 纯净性检查 ---")
if bad:
    print(f"  失败：{len(bad)} 行不是合法 JSON（说明 stdout 被日志污染）")
    for line, e in bad:
        print(f"    {line!r} -> {e}")
else:
    print(f"  通过：{len(out_lines)} 行全部是合法 JSON-RPC")

# ---- Assertion 2: initialize returns server information ----
init = next((r for r in responses if r.get("id") == 1), None)
print("\n--- initialize ---")
if init and "result" in init:
    si = init["result"].get("serverInfo", {})
    print(f"  通过：serverInfo = {si}")
else:
    print(f"  失败：{init}")

# ---- Assertion 3: tools/list returns the expected tools ----
tools_resp = next((r for r in responses if r.get("id") == 2), None)
print("\n--- tools/list ---")
expected = {"inspect_port", "scan_dev_ports", "free_port", "clean_current_project"}
if tools_resp and "result" in tools_resp:
    tools = tools_resp["result"].get("tools", [])
    names = {t["name"] for t in tools}
    print(f"  返回工具数：{len(tools)}")
    for t in tools:
        ann = t.get("annotations", {})
        print(
            f"    - {t['name']:<16} title={ann.get('title')!r} "
            f"readOnly={ann.get('readOnlyHint')} destructive={ann.get('destructiveHint')}"
        )
    print(f"  期望工具齐全：{expected == names}")
    if expected != names:
        print(f"    缺失 {expected - names}，多出 {names - expected}")
else:
    print(f"  失败：{tools_resp}")

# ---- Assertion 4: startup messages on stderr do not affect the protocol ----
print("\n--- stderr（允许存在，属于正常诊断输出）---")
for line in err_text.splitlines()[:4]:
    print(f"  {line}")

ok = (not bad) and init and tools_resp and expected == {t["name"] for t in (tools_resp.get("result", {}).get("tools", []))}
print(f"\n=== 总体结论: {'PASS' if ok else 'FAIL'} ===")
sys.exit(0 if ok else 1)
