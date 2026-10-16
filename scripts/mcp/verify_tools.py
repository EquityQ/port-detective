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
        "找不到二进制 %s\n请先执行 `go build -o port-detective .`，"
        "或用环境变量 PORT_DETECTIVE_BIN 指定路径。\n" % _BIN)
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
    return r.get("isError"), content[0].get("text", "<无>")


out = []


def show(label, resp_id, limit=600):
    is_err, text = tool_text(resp_id)
    out.append(f"\n=== {label} ===")
    out.append(f"  isError={is_err}")
    out.append("  " + (text if len(text) <= limit else text[:limit] + " …(截断)"))


show("① inspect_port(1)  受控系统端口探测 — 应返回合法诊断", 2)
show("② scan_dev_ports()  默认仅覆盖开发常用端口", 3)
show("③ inspect_port(70000)  越界端口 — 应 isError=True", 4)
show("④ inspect_port(8080)  空闲端口 — 应 is_open=False", 5)

out.append("\n=== stdout 纯净性检查 ===")
out.append(f"  非法 JSON 行数 = {len(bad)}  ->  {'通过' if not bad else '失败 ' + str(bad)}")
out.append(f"  收到响应数 = {len(responses)} (期望 5)")

err4 = tool_text(4)[0]
err_lines = [l for l in proc.stderr.decode("utf-8", "replace").splitlines() if l.strip()]
out.append(f"\n=== stderr（诊断输出，允许存在）{len(err_lines)} 行 ===")
out.append("  " + (err_lines[0] if err_lines else "<空>"))

sys.stdout.buffer.write("\n".join(out).encode("utf-8"))
sys.stdout.buffer.flush()

ok = (not bad) and len(responses) == 5 and err4 is True
sys.exit(0 if ok else 1)
