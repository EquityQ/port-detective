#!/usr/bin/env python3
"""Run local cross-language quality checks for port-detective."""

from __future__ import annotations

import argparse
import importlib.util
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
NODE_DIR = ROOT / "npm" / "port-detective"


def run_step(
    title: str,
    command: list[str],
    *,
    cwd: Path = ROOT,
    env: dict[str, str] | None = None,
) -> None:
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


def resolve_go(*, needed_for: str) -> tuple[str, str, dict[str, str]]:
    env = os.environ.copy()
    go_override = env.get("GO_BINARY", "").strip()
    go_bin = go_override or shutil.which("go")
    if not go_bin:
        raise SystemExit(
            f"go executable not found ({needed_for}); install Go 1.25.5+, set GO_BINARY, "
            "or pass --skip-go --skip-python --skip-node"
        )
    go_path = Path(go_bin)
    if go_path.parent != Path(".") or go_path.is_file():
        go_dir = str(go_path.resolve().parent)
        env["PATH"] = go_dir + os.pathsep + env.get("PATH", "")
    else:
        go_dir = str(Path(shutil.which(go_bin) or go_bin).resolve().parent)
    gofmt_name = "gofmt.exe" if os.name == "nt" else "gofmt"
    gofmt_candidate = Path(go_dir) / gofmt_name
    gofmt_bin = (
        str(gofmt_candidate)
        if gofmt_candidate.is_file()
        else (shutil.which("gofmt") or "gofmt")
    )
    return go_bin, gofmt_bin, env


def find_tsc() -> str | None:
    local_name = "tsc.cmd" if os.name == "nt" else "tsc"
    local_tsc = NODE_DIR / "node_modules" / ".bin" / local_name
    if local_tsc.is_file():
        return str(local_tsc)
    return shutil.which("tsc")


def run_mcp_acceptance(go_bin: str, go_env: dict[str, str]) -> None:
    temp_dir = Path(tempfile.mkdtemp(prefix="pd-checks-"))
    try:
        binary_name = "port-detective.exe" if os.name == "nt" else "port-detective"
        binary_path = temp_dir / binary_name
        run_step(
            "Build CLI binary for MCP acceptance checks",
            [go_bin, "build", "-o", str(binary_path), "."],
            env=go_env,
        )
        mcp_env = go_env.copy()
        mcp_env["PORT_DETECTIVE_BIN"] = str(binary_path)
        run_step(
            "MCP handshake acceptance check",
            [sys.executable, "scripts/mcp/verify_handshake.py"],
            env=mcp_env,
        )
        run_step(
            "MCP tools acceptance check",
            [sys.executable, "scripts/mcp/verify_tools.py"],
            env=mcp_env,
        )
        run_step(
            "MCP stdout purity acceptance check",
            [sys.executable, "scripts/mcp/verify_stdout_purity.py"],
            env=mcp_env,
        )
    finally:
        shutil.rmtree(temp_dir, ignore_errors=True)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--skip-go",
        action="store_true",
        help="skip Go build, vet, gofmt, race tests, and MCP acceptance",
    )
    parser.add_argument(
        "--skip-python",
        action="store_true",
        help="skip Python SDK, mypy, and release script tests",
    )
    parser.add_argument(
        "--skip-node",
        action="store_true",
        help="skip Node.js client and TypeScript declaration checks",
    )
    parser.add_argument(
        "--skip-mypy",
        action="store_true",
        help="skip Python mypy type check",
    )
    parser.add_argument(
        "--skip-tsc",
        action="store_true",
        help="skip Node.js TypeScript declaration check",
    )
    parser.add_argument(
        "--skip-mcp",
        action="store_true",
        help="skip MCP Stdio acceptance checks",
    )
    parser.add_argument(
        "--strict",
        action="store_true",
        help="fail if optional linters/typecheckers (mypy, tsc) are not installed",
    )
    args = parser.parse_args()

    go_env = os.environ.copy()
    go_bin = "go"
    gofmt_bin = "gofmt"
    need_go_for_sdk = (not args.skip_python) or (not args.skip_node)
    if not args.skip_go or need_go_for_sdk:
        reason = (
            "needed for Go checks"
            if not args.skip_go
            else "Python/Node SDK fixture tests compile testdata/sdk-cli/main.go"
        )
        go_bin, gofmt_bin, go_env = resolve_go(needed_for=reason)

    if not args.skip_go:
        run_step("Go build", [go_bin, "build", "./..."], env=go_env)
        run_step("Go vet", [go_bin, "vet", "./..."], env=go_env)
        check_gofmt(gofmt_bin)
        run_step("Go tests (-race)", [go_bin, "test", "-race", "./..."], env=go_env)
        if not args.skip_mcp:
            run_mcp_acceptance(go_bin, go_env)
    elif not args.skip_mcp:
        print("==> SKIP: MCP acceptance checks (--skip-go)", flush=True)

    run_step(
        "API reference drift check",
        [sys.executable, "scripts/quality/verify_api_reference.py"],
        env=go_env,
    )

    if not args.skip_python:
        if not args.skip_mypy:
            if importlib.util.find_spec("mypy") is not None:
                run_step(
                    "Python mypy check",
                    [sys.executable, "-m", "mypy", "python/port_detective"],
                    env=go_env,
                )
            elif args.strict:
                raise SystemExit("mypy is not installed; install mypy or omit --strict")
            else:
                print(
                    "==> SKIP: mypy not installed (pass --strict to require it)",
                    flush=True,
                )
        run_step(
            "Python SDK tests",
            [sys.executable, "-m", "unittest", "discover", "-s", "python/tests", "-q"],
            env=go_env,
        )
        run_step(
            "Release script tests",
            [
                sys.executable,
                "-m",
                "unittest",
                "discover",
                "-s",
                "scripts/release",
                "-p",
                "test_*.py",
                "-q",
            ],
            env=go_env,
        )

    if not args.skip_node:
        npm_bin = shutil.which("npm")
        if not npm_bin:
            raise SystemExit("npm executable not found; install Node.js 22+ or pass --skip-node")
        if not args.skip_tsc:
            tsc_bin = find_tsc()
            if tsc_bin is not None:
                run_step(
                    "Node.js TypeScript declaration check",
                    [
                        tsc_bin,
                        "--noEmit",
                        "--strict",
                        "--target",
                        "ES2022",
                        "--module",
                        "CommonJS",
                        "index.d.ts",
                    ],
                    cwd=NODE_DIR,
                    env=go_env,
                )
            elif args.strict:
                raise SystemExit("tsc is not installed; install TypeScript or omit --strict")
            else:
                print(
                    "==> SKIP: tsc not installed (pass --strict to require it)",
                    flush=True,
                )
        run_step(
            "Node.js client tests",
            [npm_bin, "test", "--prefix", "npm/port-detective"],
            env=go_env,
        )

    print("All selected quality checks passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
