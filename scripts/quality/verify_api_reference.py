#!/usr/bin/env python3
"""Verify that the CLI/MCP API reference and SDK schemas match the implementation."""

from __future__ import annotations

import importlib.util
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
REFERENCE = ROOT / "docs" / "project" / "API_REFERENCE.md"
GENERATOR = ROOT / "scripts" / "quality" / "generate_api_reference.py"
PYTHON_MODELS = ROOT / "python" / "port_detective" / "models.py"
NODE_TYPES = ROOT / "npm" / "port-detective" / "index.d.ts"
NODE_INDEX = ROOT / "npm" / "port-detective" / "index.js"


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


def verify_sdk_schema_parity(diagnostic_fields: list[str]) -> None:
    if not diagnostic_fields:
        fail("could not extract PortDiagnostic JSON fields from internal/core")

    py_source = PYTHON_MODELS.read_text(encoding="utf-8")
    py_match = re.search(r"class\s+PortDiagnostic:.*?(?=\n@dataclass|\Z)", py_source, flags=re.DOTALL)
    if py_match is None:
        fail("PortDiagnostic dataclass not found in python/port_detective/models.py")
    py_block = py_match.group(0)
    missing_py = [f for f in diagnostic_fields if not re.search(rf"(?m)^\s+{re.escape(f)}\s*:", py_block)]
    if missing_py:
        fail(f"Python PortDiagnostic missing fields: {', '.join(missing_py)}")

    ts_source = NODE_TYPES.read_text(encoding="utf-8")
    ts_match = re.search(r"export\s+interface\s+PortDiagnostic\s*\{(.*?)\n\}", ts_source, flags=re.DOTALL)
    if ts_match is None:
        fail("PortDiagnostic interface not found in npm/port-detective/index.d.ts")
    ts_block = ts_match.group(1)
    missing_ts = [f for f in diagnostic_fields if not re.search(rf"(?m)^\s+{re.escape(f)}\??\s*:", ts_block)]
    if missing_ts:
        fail(f"TypeScript PortDiagnostic missing fields: {', '.join(missing_ts)}")

    js_source = NODE_INDEX.read_text(encoding="utf-8")
    export_match = re.search(r"module\.exports\s*=\s*\{(.*?)\};", js_source, flags=re.DOTALL)
    if export_match is not None:
        exported_names = re.findall(r"\b([A-Za-z_][A-Za-z0-9_]*)\b", export_match.group(1))
        missing_exports = [
            name for name in exported_names
            if not re.search(rf"\bexport\s+(?:function|const|class)\s+{re.escape(name)}\b", ts_source)
        ]
        if missing_exports:
            fail(f"TypeScript index.d.ts missing exports from index.js: {', '.join(sorted(set(missing_exports)))}")


def main() -> int:
    reference = REFERENCE.read_text(encoding="utf-8")
    generator = load_generator()
    manifest = generator.build_manifest()

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
        diagnostic_fields = list(manifest["core"]["diagnostic_fields"])
    except (KeyError, TypeError) as exc:
        fail(f"generated manifest schema is invalid: {exc}")
    require_tokens(reference, {f"`{item['name']}" for item in commands}, "CLI commands")
    require_tokens(reference, {f"--{item['name']}" for item in flags}, "CLI flags")
    require_tokens(reference, {f"`{tool['name']}`" for tool in tools}, "MCP tools")
    require_tokens(reference, {f"`{argument}`" for argument in arguments}, "MCP arguments")
    require_tokens(reference, {f'"{field}"' for field in diagnostic_fields}, "PortDiagnostic JSON fields")
    require_tokens(reference, core_terms, "contract terms")
    verify_sdk_schema_parity(diagnostic_fields)
    print(
        "API reference and SDK schemas match "
        f"{len(commands)} CLI commands, {len(flags)} CLI flags, "
        f"{len(tools)} MCP tools, {len(arguments)} MCP arguments, and "
        f"{len(diagnostic_fields)} PortDiagnostic JSON fields."
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
