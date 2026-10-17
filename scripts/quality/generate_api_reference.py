#!/usr/bin/env python3
"""Generate the checked-in machine-readable CLI/MCP API manifest."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CLI_DIR = ROOT / "internal" / "cli"
MCP_SOURCE = ROOT / "internal" / "mcpserver" / "server.go"
OUTPUT = ROOT / "docs" / "project" / "API_REFERENCE.generated.json"
MARKDOWN_OUTPUT = ROOT / "docs" / "project" / "API_REFERENCE.generated.md"

SCHEMA_VERSION = 1
EXPECTED_CORE_TYPES = ("PortDiagnostic",)
EXPECTED_CORE_CONSTANTS = ("DefaultDevPorts",)
EXPECTED_CORE_FUNCTIONS = ("InspectPort", "ScanPortsConcurrent", "KillPortProcess")
REFERENCE_TERMS = ("JSON-RPC", "completion <shell>")


def relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def read(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def go_sources(directory: Path) -> list[Path]:
    return sorted(path for path in directory.glob("*.go") if not path.name.endswith("_test.go"))


def extract_cli() -> dict[str, list[dict[str, Any]]]:
    commands: dict[str, dict[str, Any]] = {}
    flags: dict[str, dict[str, Any]] = {}

    for path in go_sources(CLI_DIR):
        source = read(path)
        source_name = relative(path)
        for match in re.finditer(r'(?m)^\s*Use:\s*"([^"]+)"', source):
            use = match.group(1)
            name = use.split(maxsplit=1)[0]
            if name == "port-detective" or not re.fullmatch(r"[a-z][a-z0-9-]*", name):
                continue
            commands[name] = {"name": name, "use": use, "source": source_name}

        flag_pattern = (
            r'\b(?:PersistentFlags|Flags)\(\)\.'
            r'(?:StringVar|StringVarP|BoolVar|BoolVarP|IntVar|IntVarP|'
            r'Float64Var|Float64VarP)\(\s*&?[\w.]+,\s*"([a-z][a-z0-9-]*)"'
        )
        for match in re.finditer(flag_pattern, source):
            name = match.group(1)
            flags[name] = {"name": name, "source": source_name}

    return {
        "commands": [commands[name] for name in sorted(commands)],
        "flags": [flags[name] for name in sorted(flags)],
    }


def extract_mcp() -> list[dict[str, Any]]:
    source = read(MCP_SOURCE)
    tools: list[dict[str, Any]] = []
    function_pattern = re.compile(
        r"(?m)^func\s+(\w+Tool)\(\)\s+mcp\.Tool\s*\{"
    )
    matches = list(function_pattern.finditer(source))
    for index, match in enumerate(matches):
        end = matches[index + 1].start() if index + 1 < len(matches) else len(source)
        body = source[match.start() : end]
        tool_match = re.search(r'mcp\.NewTool\("([a-z][a-z0-9_]*)"', body)
        if tool_match is None:
            continue
        arguments = sorted(
            {
                argument
                for argument in re.findall(
                    r'mcp\.With(?:Number|String|Boolean)\("([a-z][a-z0-9_]*)"',
                    body,
                )
            }
        )
        tools.append(
            {
                "name": tool_match.group(1),
                "source": relative(MCP_SOURCE),
                "arguments": arguments,
            }
        )
    return sorted(tools, key=lambda tool: tool["name"])


def extract_core() -> dict[str, list[str]]:
    core_sources = go_sources(ROOT / "internal" / "core")
    source = "\n".join(read(path) for path in core_sources)

    def declared(pattern: str, names: tuple[str, ...]) -> list[str]:
        return [name for name in names if re.search(pattern.format(name=re.escape(name)), source)]

    return {
        "types": declared(r"\btype\s+{name}\s+struct\b", EXPECTED_CORE_TYPES),
        "constants": declared(r"\b(?:var|const)\s+{name}\b", EXPECTED_CORE_CONSTANTS),
        "functions": declared(r"\bfunc\s+{name}\s*\(", EXPECTED_CORE_FUNCTIONS),
    }


def build_manifest() -> dict[str, Any]:
    cli = extract_cli()
    mcp = extract_mcp()
    core = extract_core()
    missing_core = {
        category: sorted(set(expected) - set(core[category]))
        for category, expected in (
            ("types", EXPECTED_CORE_TYPES),
            ("constants", EXPECTED_CORE_CONSTANTS),
            ("functions", EXPECTED_CORE_FUNCTIONS),
        )
    }
    missing_core = {category: names for category, names in missing_core.items() if names}
    if missing_core:
        raise ValueError(f"frozen core API declarations are missing: {missing_core}")

    return {
        "schema_version": SCHEMA_VERSION,
        "generator": "scripts/quality/generate_api_reference.py",
        "source_files": [
            *(relative(path) for path in go_sources(CLI_DIR)),
            *(relative(path) for path in go_sources(ROOT / "internal" / "core")),
            relative(MCP_SOURCE),
        ],
        "cli": cli,
        "mcp": {"source": relative(MCP_SOURCE), "tools": mcp},
        "core": core,
        "reference_terms": list(REFERENCE_TERMS),
    }


def render_markdown(manifest: dict[str, Any]) -> str:
    lines = [
        "# Generated API Inventory / 自动生成的 API 清单",
        "",
        "Generated from the CLI, core, and MCP implementation. See",
        "[API_REFERENCE.md](API_REFERENCE.md) for behavior, result shapes, and safety rules.",
        "",
        "根据 CLI、core 和 MCP 实现生成。行为、结果结构和安全规则见",
        "[API_REFERENCE.md](API_REFERENCE.md)。",
        "",
        "## CLI commands / CLI 命令",
        "",
        "| Command / 命令 | Syntax / 用法 | Source / 源文件 |",
        "| --- | --- | --- |",
    ]
    for command in manifest["cli"]["commands"]:
        syntax = command["use"].replace("|", "\\|")
        lines.append(f"| `{command['name']}` | `{syntax}` | `{command['source']}` |")

    lines.extend(
        [
            "",
            "## CLI flags / CLI 参数",
            "",
            "| Flag / 参数 | Source / 源文件 |",
            "| --- | --- |",
        ]
    )
    for flag in manifest["cli"]["flags"]:
        lines.append(f"| `--{flag['name']}` | `{flag['source']}` |")

    lines.extend(
        [
            "",
            "## MCP tools / MCP 工具",
            "",
            "| Tool / 工具 | Arguments / 参数 | Source / 源文件 |",
            "| --- | --- | --- |",
        ]
    )
    for tool in manifest["mcp"]["tools"]:
        arguments = ", ".join(f"`{argument}`" for argument in tool["arguments"]) or "None"
        lines.append(f"| `{tool['name']}` | {arguments} | `{tool['source']}` |")

    core = manifest["core"]
    lines.extend(
        [
            "",
            "## Core API / 核心 API",
            "",
            "| Kind / 类型 | Names / 名称 |",
            "| --- | --- |",
            f"| Types | {', '.join(f'`{name}`' for name in core['types'])} |",
            f"| Values | {', '.join(f'`{name}`' for name in core['constants'])} |",
            f"| Functions | {', '.join(f'`{name}`' for name in core['functions'])} |",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    manifest = build_manifest()
    OUTPUT.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    MARKDOWN_OUTPUT.write_text(render_markdown(manifest), encoding="utf-8")
    print(f"Generated {relative(OUTPUT)} and {relative(MARKDOWN_OUTPUT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
