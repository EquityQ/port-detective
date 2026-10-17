#!/usr/bin/env python3
"""Run a safe end-to-end CLI demo against a temporary local TCP listener."""

from __future__ import annotations

import argparse
import json
import os
import socket
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def run(binary: str, *args: str) -> str:
    result = subprocess.run(
        [binary, *args, "--json"],
        check=False,
        capture_output=True,
        text=True,
        encoding="utf-8",
        timeout=30,
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"command failed ({result.returncode}): {binary} {' '.join(args)}\n"
            f"{result.stderr.strip()}"
        )
    return result.stdout


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--binary",
        default=os.environ.get("PORT_DETECTIVE_BIN"),
        help="CLI executable path (defaults to the repository build artifact)",
    )
    args = parser.parse_args()
    binary = args.binary or str(
        ROOT / ("port-detective.exe" if os.name == "nt" else "port-detective")
    )
    if not Path(binary).is_file():
        parser.error(
            f"CLI binary not found: {binary}; build it with `go build -o "
            "port-detective .` or pass --binary"
        )

    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        listener.listen(8)
        port = listener.getsockname()[1]

        diagnostic = json.loads(run(binary, "check", str(port)))
        if not diagnostic.get("is_open"):
            raise RuntimeError(f"temporary listener on port {port} was not detected")
        print(f"check: port {port} is open (PID {diagnostic.get('pid', 0)})")

        scan = json.loads(run(binary, "scan", "--ports", str(port)))
        if not any(item.get("port") == port for item in scan):
            raise RuntimeError(f"scan did not return temporary listener on port {port}")
        print(f"scan: port {port} appears in the occupied-port results")

        preview = json.loads(run(binary, "kill", str(port), "--dry-run"))
        if not preview.get("dry_run"):
            raise RuntimeError("kill command did not preserve dry-run mode")
        listener.settimeout(2)
        with socket.create_connection(("127.0.0.1", port), timeout=2):
            pass
        print("kill --dry-run: preview returned; temporary listener remains open")

    print("demo complete: no external process was terminated")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, RuntimeError, subprocess.SubprocessError, json.JSONDecodeError) as exc:
        print(f"demo failed: {exc}", file=sys.stderr)
        raise SystemExit(1) from exc
