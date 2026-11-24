#!/usr/bin/env python3
"""Run local cross-language quality checks for port-detective."""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def run_step(title: str, command: list[str], *, cwd: Path = ROOT, env: dict[str, str] | None = None) -> None:
    print(f"==> {title}", flush=True)
    result = subprocess.run(command, cwd=cwd, env=env, check=False)
    if result.returncode != 0:
        raise SystemExit(f"check failed ({result.returncode}): {title}")


def check_gofmt(gofmt_bin: str) -> None:
    print("==> Check Go formatting", flush=True)
    listed = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "*.go"],
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=True,
    )
    files = [
        line.strip()
        for line in listed.stdout.splitlines()
        if line.strip() and (ROOT / line.strip()).is_file()
    ]
    if not files:
        return
    formatted = subprocess.run(
        [gofmt_bin, "-l", *files],
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=True,
    )
    unformatted = [line.strip() for line in formatted.stdout.splitlines() if line.strip()]
    if unformatted:
        raise SystemExit("gofmt needed on:\n  " + "\n  ".join(unformatted))


def resolve_go() -> tuple[str, str, dict[str, str]]:
    env = os.environ.copy()
    go_bin = env.get("GO_BINARY") or shutil.which("go")
    if not go_bin:
        raise SystemExit("go executable not found; install Go 1.25.5+ or set GO_BINARY")
    go_dir = str(Path(go_bin).resolve().parent)
    env["PATH"] = go_dir + os.pathsep + env.get("PATH", "")
    gofmt_name = "gofmt.exe" if os.name == "nt" else "gofmt"
    gofmt_candidate = Path(go_dir) / gofmt_name
    gofmt_bin = str(gofmt_candidate) if gofmt_candidate.is_file() else (shutil.which("gofmt") or "gofmt")
    return go_bin, gofmt_bin, env


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--skip-go", action="store_true", help="skip Go build, vet, gofmt, and race tests")
    parser.add_argument("--skip-python", action="store_true", help="skip Python SDK and release script tests")
    parser.add_argument("--skip-node", action="store_true", help="skip Node.js client tests")
    args = parser.parse_args()

    go_env = os.environ.copy()
    if not args.skip_go or not args.skip_python or not args.skip_node:
        go_bin, gofmt_bin, go_env = resolve_go()
    else:
        go_bin, gofmt_bin = "go", "gofmt"

    if not args.skip_go:
        run_step("Go build", [go_bin, "build", "./..."], env=go_env)
        run_step("Go vet", [go_bin, "vet", "./..."], env=go_env)
        check_gofmt(gofmt_bin)
        run_step("Go tests (-race)", [go_bin, "test", "-race", "./..."], env=go_env)

    run_step("API reference drift check", [sys.executable, "scripts/quality/verify_api_reference.py"], env=go_env)

    if not args.skip_python:
        run_step(
            "Python SDK tests",
            [sys.executable, "-m", "unittest", "discover", "-s", "python/tests", "-q"],
            env=go_env,
        )
        run_step(
            "Release script tests",
            [sys.executable, "-m", "unittest", "discover", "-s", "scripts/release", "-p", "test_*.py", "-q"],
            env=go_env,
        )

    if not args.skip_node:
        npm_bin = shutil.which("npm")
        if not npm_bin:
            raise SystemExit("npm executable not found; install Node.js 22+ or pass --skip-node")
        run_step("Node.js client tests", [npm_bin, "test", "--prefix", "npm/port-detective"], env=go_env)

    print("All selected quality checks passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
