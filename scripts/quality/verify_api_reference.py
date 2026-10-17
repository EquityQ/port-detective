#!/usr/bin/env python3
"""Verify that the checked-in CLI/MCP API reference matches the implementation."""

from __future__ import annotations

import importlib.util
import json
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REFERENCE = ROOT / "docs" / "project" / "API_REFERENCE.md"
MANIFEST = ROOT / "docs" / "project" / "API_REFERENCE.generated.json"
GENERATED_REFERENCE = ROOT / "docs" / "project" / "API_REFERENCE.generated.md"
GENERATOR = ROOT / "scripts" / "quality" / "generate_api_reference.py"


def fail(message: str) -> None:
    print(f"API reference drift: {message}", file=sys.stderr)
    raise SystemExit(1)


def require_tokens(document: str, tokens: set[str], label: str) -> None:
    missing = sorted(token for token in tokens if token not in document)
    if missing:
        fail(f"{label} missing {', '.join(missing)}")


def load_generator():
    spec = importlib.util.spec_from_file_location("api_reference_generator", GENERATOR)
    if spec is None or spec.loader is None:
        fail(f"cannot load generator {GENERATOR}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main() -> int:
    reference = REFERENCE.read_text(encoding="utf-8")
    try:
        manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read generated manifest: {exc}")

    generator = load_generator()
    expected = generator.build_manifest()
    if manifest != expected:
        fail("generated manifest is stale; run python scripts/quality/generate_api_reference.py")
    try:
        generated_reference = GENERATED_REFERENCE.read_text(encoding="utf-8")
    except OSError as exc:
        fail(f"cannot read generated Markdown inventory: {exc}")
    if generated_reference != generator.render_markdown(expected):
        fail("generated Markdown inventory is stale; run python scripts/quality/generate_api_reference.py")

    try:
        commands = manifest["cli"]["commands"]
        flags = manifest["cli"]["flags"]
        tools = manifest["mcp"]["tools"]
        arguments = {
            argument
            for tool in tools
            for argument in tool["arguments"]
        }
        core_terms = set(manifest["core"]["types"])
        core_terms.update(manifest["core"]["constants"])
        core_terms.update(manifest["core"]["functions"])
        core_terms.update(manifest["reference_terms"])
    except (KeyError, TypeError) as exc:
        fail(f"generated manifest schema is invalid: {exc}")
    require_tokens(reference, {f"`{item['name']}" for item in commands}, "CLI commands")
    require_tokens(reference, {f"--{item['name']}" for item in flags}, "CLI flags")
    require_tokens(reference, {f"`{tool['name']}`" for tool in tools}, "MCP tools")
    require_tokens(reference, {f"`{argument}`" for argument in arguments}, "MCP arguments")
    require_tokens(reference, core_terms, "contract terms")
    require_tokens(reference, {"API_REFERENCE.generated.json", "API_REFERENCE.generated.md"}, "generated reference links")
    print(
        "API reference matches "
        f"{len(commands)} CLI commands, {len(flags)} CLI flags, "
        f"{len(tools)} MCP tools, and {len(arguments)} MCP argument names."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
