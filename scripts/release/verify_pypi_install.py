"""Install a published Python SDK version and exercise its CLI resolution."""

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Optional


def fail(message: str) -> None:
    raise SystemExit(message)


def install_package(target: Path, version: str, index_url: str, attempts: int) -> None:
    last_output = ""
    for attempt in range(1, attempts + 1):
        if target.exists():
            shutil.rmtree(target)
        target.mkdir(parents=True)
        try:
            result = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "pip",
                    "install",
                    "--disable-pip-version-check",
                    "--no-input",
                    "--no-cache-dir",
                    "--no-deps",
                    "--index-url",
                    index_url,
                    "--target",
                    str(target),
                    f"port-detective=={version}",
                ],
                capture_output=True,
                text=True,
                encoding="utf-8",
                check=False,
                timeout=120,
            )
        except subprocess.TimeoutExpired as exc:
            last_output = f"pip install timed out after 120 seconds: {exc.stderr or ''}"
        else:
            if result.returncode == 0:
                return
            last_output = (result.stderr or result.stdout).strip()
        if attempt < attempts:
            time.sleep(min(5 * attempt, 30))
    fail(f"could not install port-detective=={version} from {index_url}: {last_output}")


def verify_import(target: Path, version: str, cli: Optional[Path]) -> None:
    code = """
import os
import socket
from port_detective import __version__, inspect_port

assert __version__ == os.environ["EXPECTED_VERSION"], (__version__, os.environ["EXPECTED_VERSION"])
listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
listener.bind(("127.0.0.1", 0))
listener.listen(1)
try:
    diagnostic = inspect_port(listener.getsockname()[1], timeout=20)
    assert diagnostic.is_open, diagnostic
finally:
    listener.close()
"""
    env = os.environ.copy()
    existing_pythonpath = env.get("PYTHONPATH")
    env["PYTHONPATH"] = os.pathsep.join(
        part for part in (str(target), existing_pythonpath) if part
    )
    if cli is not None:
        env["PORT_DETECTIVE_BIN"] = str(cli.resolve())
        env["PORT_DETECTIVE_AUTO_DOWNLOAD"] = "0"
    else:
        env.pop("PORT_DETECTIVE_BIN", None)
        env["PORT_DETECTIVE_AUTO_DOWNLOAD"] = "1"
        env["PORT_DETECTIVE_CACHE_DIR"] = str(target.parent / "cli-cache")
    env["EXPECTED_VERSION"] = version
    result = subprocess.run(
        [sys.executable, "-c", code],
        capture_output=True,
        text=True,
        encoding="utf-8",
        env=env,
        check=False,
    )
    if result.returncode != 0:
        fail(f"published SDK verification failed: {(result.stderr or result.stdout).strip()}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--cli", type=Path, help="Use a local CLI instead of the first-use download")
    parser.add_argument("--index-url", default="https://pypi.org/simple")
    parser.add_argument("--attempts", type=int, default=12)
    args = parser.parse_args()
    if args.attempts < 1:
        fail("--attempts must be positive")
    if args.cli is not None and not args.cli.is_file():
        fail(f"CLI binary not found: {args.cli}")

    with tempfile.TemporaryDirectory(prefix="port-detective-pypi-") as temp_dir:
        target = Path(temp_dir) / "site"
        install_package(target, args.version, args.index_url, args.attempts)
        verify_import(target, args.version, args.cli)
    print(f"verified PyPI installation and CLI integration for port-detective {args.version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
