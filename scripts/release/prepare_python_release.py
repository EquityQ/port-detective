"""Set the Python package version for a tagged release build."""

from __future__ import annotations

import argparse
import re
from pathlib import Path


def replace_once(path: Path, pattern: str, replacement: str) -> None:
    text = path.read_text(encoding="utf-8")
    updated, count = re.subn(pattern, replacement, text, count=1, flags=re.MULTILINE)
    if count != 1:
        raise SystemExit(f"expected one version declaration in {path}")
    path.write_text(updated, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True, help="Python package root")
    parser.add_argument("--version", required=True, help="Release version without the leading v")
    args = parser.parse_args()

    if not re.fullmatch(r"\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?", args.version):
        raise SystemExit(f"invalid release version: {args.version}")

    replace_once(
        args.root / "pyproject.toml",
        r'^(version\s*=\s*["\'])[^"\']+(["\'])$',
        rf'\g<1>{args.version}\g<2>',
    )
    replace_once(
        args.root / "port_detective" / "__init__.py",
        r'^(__version__\s*=\s*["\'])[^"\']+(["\'])$',
        rf'\g<1>{args.version}\g<2>',
    )
    print(f"prepared Python package version {args.version}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
