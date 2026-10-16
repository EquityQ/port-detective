#!/usr/bin/env python3
"""Verify that the checked-in CLI/MCP API reference matches the implementation."""

from __future__ import annotations

import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REFERENCE = ROOT / "docs" / "project" / "API_REFERENCE.md"
CLI_SOURCE = ROOT / "internal" / "cli" / "root.go"
MCP_SOURCE = ROOT / "internal" / "mcpserver" / "server.go"


def fail(message: str) -> None:
    print(f"API reference drift: {message}", file=sys.stderr)
    raise SystemExit(1)


def require_tokens(document: str, tokens: set[str], label: str) -> None:
    missing = sorted(token for token in tokens if token not in document)
    if missing:
        fail(f"{label} missing {', '.join(missing)}")


def main() -> int:
    reference = REFERENCE.read_text(encoding="utf-8")
    cli = CLI_SOURCE.read_text(encoding="utf-8")
    mcp = MCP_SOURCE.read_text(encoding="utf-8")

    commands = {
        match.group(1)
        for match in re.finditer(r'\bUse:\s+"([a-z][a-z0-9-]*)', cli)
        if match.group(1) != "port-detective"
    }
    expected_commands = {"check", "scan", "kill", "clean", "mcp"}
    if commands != expected_commands:
        fail(f"CLI commands are {sorted(commands)}, expected {sorted(expected_commands)}")
    require_tokens(reference, {f"`{command}" for command in commands}, "CLI commands")

    flags = {
        match.group(1)
        for match in re.finditer(
            r'\b(?:PersistentFlags|Flags)\(\)\.(?:StringVar|StringVarP|BoolVar|BoolVarP)\(\s*&?[\w.]+,\s*"([a-z-]+)"',
            cli,
        )
    }
    expected_flags = {"json", "lang", "ports", "dry-run", "yes"}
    if not expected_flags.issubset(flags):
        fail(f"CLI flags are missing from implementation: {sorted(expected_flags - flags)}")
    require_tokens(reference, {f"--{flag}" for flag in expected_flags}, "CLI flags")

    tools = set(re.findall(r'mcp\.NewTool\("([a-z_]+)"', mcp))
    expected_tools = {"inspect_port", "scan_dev_ports", "free_port", "clean_current_project"}
    if tools != expected_tools:
        fail(f"MCP tools are {sorted(tools)}, expected {sorted(expected_tools)}")
    require_tokens(reference, {f"`{tool}`" for tool in tools}, "MCP tools")

    arguments = set(
        re.findall(r'mcp\.With(?:Number|String|Boolean)\("([a-z_]+)"', mcp)
    )
    expected_arguments = {"port", "ports", "dry_run"}
    if arguments != expected_arguments:
        fail(
            f"MCP arguments are {sorted(arguments)}, "
            f"expected {sorted(expected_arguments)}"
        )
    require_tokens(reference, {f"`{argument}`" for argument in arguments}, "MCP arguments")

    require_tokens(
        reference,
        {"PortDiagnostic", "`dry_run`", "JSON-RPC", "DefaultDevPorts"},
        "contract terms",
    )
    print(
        "API reference matches "
        f"{len(commands)} CLI commands, {len(tools)} MCP tools, "
        f"and {len(arguments)} MCP argument names."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
