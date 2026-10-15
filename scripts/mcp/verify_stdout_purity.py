#!/usr/bin/env python3
"""Verify that the MCP startup path does not write non-protocol output to os.Stdout.

Procedure:
  1. Send one initialize request and close stdin so the server exits naturally.
  2. Assert stdout contains exactly one line with a valid JSON-RPC response;
  3. Count stderr output; diagnostics are allowed only there.
"""
import json
import os
import subprocess
import sys

# Binary resolution order: PORT_DETECTIVE_BIN, then the repository build artifact.
# On Windows, the binary is port-detective.exe; on other platforms it is port-detective.
_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_BIN = os.environ.get("PORT_DETECTIVE_BIN") or os.path.join(
    _ROOT, "port-detective.exe" if os.name == "nt" else "port-detective")
if not os.path.exists(_BIN):
    sys.stderr.write(
        "找不到二进制 %s\n请先执行 `go build -o port-detective .`，"
        "或用环境变量 PORT_DETECTIVE_BIN 指定路径。\n" % _BIN)
    sys.exit(2)
BIN = _BIN

payload = json.dumps({
    "jsonrpc": "2.0", "id": 1, "method": "initialize",
    "params": {"protocolVersion": "2024-11-05", "capabilities": {},
               "clientInfo": {"name": "purity-check", "version": "1.0"}},
}) + "\n"

proc = subprocess.run([BIN, "mcp"], input=payload.encode(),
                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)

raw_stdout = proc.stdout.decode("utf-8", "replace")
raw_stderr = proc.stderr.decode("utf-8", "replace")
lines = [l for l in raw_stdout.splitlines() if l.strip()]

out = []
out.append(f"stdout 原始字节数 : {len(proc.stdout)}")
out.append(f"stdout 非空行数   : {len(lines)}   (期望恰好 1)")
out.append(f"stderr 行数       : {len([l for l in raw_stderr.splitlines() if l.strip()])}")

ok = True

# Assertion 1: stdout contains exactly one line.
if len(lines) != 1:
    ok = False
    out.append(f"\n[失败] stdout 行数应为 1，实际 {len(lines)}：")
    for l in lines[:10]:
        out.append(f"    {l[:160]}")
else:
    out.append("\n[通过] stdout 恰好 1 行")

# Assertion 2: the line is valid JSON-RPC and has the expected ID.
if lines:
    try:
        obj = json.loads(lines[0])
        if obj.get("id") != 1 or "result" not in obj:
            ok = False
            out.append(f"[失败] 不是预期的 initialize 响应：{lines[0][:200]}")
        else:
            out.append(f"[通过] 合法 JSON-RPC 响应，serverInfo="
                       f"{obj['result'].get('serverInfo')}")
    except json.JSONDecodeError as e:
        ok = False
        out.append(f"[失败] 该行不是合法 JSON：{e}")

# Assertion 3: stdout contains no human-readable text or ANSI escapes.
for marker in ["已启动", "已注册工具", "\x1b[", "等待客户端", "port-detective 已"]:
    if marker in raw_stdout:
        ok = False
        out.append(f"[失败] stdout 中混入了文案标记 {marker!r}")

if ok:
    out.append("[通过] stdout 中无任何人类可读文案或 ANSI 转义序列")

out.append(f"\n=== 结论: {'PASS' if ok else 'FAIL'} ===")
out.append("stderr 内容（允许，属正常诊断）：")
for l in raw_stderr.splitlines()[:3]:
    out.append(f"    {l}")

sys.stdout.buffer.write("\n".join(out).encode("utf-8"))
sys.stdout.buffer.flush()
sys.exit(0 if ok else 1)
