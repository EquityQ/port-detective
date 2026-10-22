"""Bounded MCP acceptance transport that keeps stdin open until replies arrive."""

import json
import queue
import subprocess
import threading


def run_requests(binary, requests, timeout=30):
    process = subprocess.Popen(
        [binary, "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    replies = queue.Queue()
    stdout, stderr = [], []

    def read_stdout():
        for line in process.stdout:
            stdout.append(line)
            replies.put(line)
        replies.put(None)

    def read_stderr():
        stderr.append(process.stderr.read())

    readers = [threading.Thread(target=read_stdout, daemon=True),
               threading.Thread(target=read_stderr, daemon=True)]
    for reader in readers:
        reader.start()
    try:
        for request in requests:
            process.stdin.write((json.dumps(request) + "\n").encode("utf-8"))
            process.stdin.flush()
            if "id" not in request:
                continue
            try:
                line = replies.get(timeout=timeout)
            except queue.Empty as exc:
                raise RuntimeError(f"MCP response {request['id']} timed out") from exc
            if line is None:
                raise RuntimeError(f"MCP server closed before response {request['id']}")
            response = json.loads(line)
            if response.get("jsonrpc") != "2.0" or response.get("id") != request["id"]:
                raise RuntimeError(f"Unexpected MCP response: {response!r}")
        process.stdin.close()
        process.wait(timeout=10)
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=10)
        for reader in readers:
            reader.join(timeout=5)
        for stream in (process.stdin, process.stdout, process.stderr):
            stream.close()
    return subprocess.CompletedProcess(
        [binary, "mcp"], process.returncode, b"".join(stdout), b"".join(stderr),
    )
