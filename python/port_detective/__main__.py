"""Explicit runtime installation and CLI environment diagnostics."""

import argparse
import subprocess
import sys
from pathlib import Path

from .core import PortDetectiveError, find_binary
from .runtime import RuntimeDownloadError, download_cli


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(prog="python -m port_detective")
    commands = parser.add_subparsers(dest="command", required=True)
    install = commands.add_parser("install", help="Download and verify the matching CLI in the user cache")
    install.add_argument("--destination", type=Path, help="Override the cache root directory")
    doctor = commands.add_parser("doctor", help="Locate a compatible CLI and run its read-only diagnostics")
    doctor.add_argument("--binary", help="Explicit CLI executable path")
    doctor.add_argument("--json", action="store_true", help="Pass JSON output through from the CLI")
    args = parser.parse_args(argv)
    try:
        if args.command == "install":
            print("Preparing the matching CLI: checking cache, downloading from GitHub if needed, and verifying SHA-256...", file=sys.stderr)
            print(download_cli(destination=args.destination))
            return 0
        binary = find_binary(args.binary)
        print(f"Using CLI: {binary}", file=sys.stderr)
        command = [binary, "doctor"]
        if args.json:
            command.append("--json")
        return subprocess.run(command, check=False, timeout=30).returncode
    except (PortDetectiveError, RuntimeDownloadError, OSError, subprocess.TimeoutExpired) as exc:
        print(f"port-detective: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
