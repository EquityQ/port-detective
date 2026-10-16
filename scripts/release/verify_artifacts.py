"""Verify release versions, checksums, and archive contents."""

from __future__ import annotations

import argparse
import hashlib
import re
import subprocess
import tarfile
import zipfile
from pathlib import Path


REQUIRED_ARCHIVE_FILES = {"README.md", "README_zh.md", "LICENSE"}


def check(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


def verify_cli(path: Path, expected: str) -> None:
    result = subprocess.run(
        [str(path), "--version"],
        capture_output=True,
        text=True,
        encoding="utf-8",
        check=False,
    )
    check(result.returncode == 0, f"CLI version command failed: {result.stderr.strip()}")
    output = f"{result.stdout}\n{result.stderr}"
    check(
        re.search(rf"(?<![0-9A-Za-z]){re.escape(expected)}(?![0-9A-Za-z])", output)
        is not None,
        f"CLI version does not contain {expected!r}: {output.strip()!r}",
    )


def read_declared_version(path: Path, pattern: str) -> str:
    match = re.search(pattern, path.read_text(encoding="utf-8"), flags=re.MULTILINE)
    check(match is not None, f"version declaration not found in {path}")
    return match.group(1)


def verify_python(root: Path, wheel_dir: Path | None, expected: str) -> None:
    pyproject_version = read_declared_version(
        root / "pyproject.toml", r'^version\s*=\s*["\']([^"\']+)["\']$'
    )
    init_version = read_declared_version(
        root / "port_detective" / "__init__.py",
        r'^__version__\s*=\s*["\']([^"\']+)["\']$',
    )
    check(pyproject_version == expected, f"pyproject version is {pyproject_version}, expected {expected}")
    check(init_version == expected, f"__version__ is {init_version}, expected {expected}")

    if wheel_dir is None:
        return
    wheels = sorted(wheel_dir.glob("*.whl"))
    check(len(wheels) == 1, f"expected one release wheel in {wheel_dir}, found {len(wheels)}")
    with zipfile.ZipFile(wheels[0]) as archive:
        metadata_names = [name for name in archive.namelist() if name.endswith(".dist-info/METADATA")]
        check(len(metadata_names) == 1, f"wheel metadata is ambiguous: {metadata_names}")
        metadata = archive.read(metadata_names[0]).decode("utf-8")
    wheel_version = read_declared_version_from_text(metadata, r"^Version:[ \t]*(\S+)[ \t]*$")
    check(wheel_version == expected, f"wheel version is {wheel_version}, expected {expected}")


def read_declared_version_from_text(text: str, pattern: str) -> str:
    normalized = text.replace("\r\n", "\n").replace("\r", "\n")
    match = re.search(pattern, normalized, flags=re.MULTILINE)
    check(match is not None, "version declaration not found in wheel metadata")
    return match.group(1)


def verify_checksums(dist: Path, checksums: Path) -> None:
    check(checksums.is_file(), f"checksum file not found: {checksums}")
    entries: list[tuple[str, str]] = []
    for line in checksums.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        match = re.fullmatch(r"([0-9a-fA-F]{64})\s+\*?(.+)", line)
        check(match is not None, f"invalid checksum line: {line!r}")
        entries.append((match.group(1).lower(), match.group(2)))
    check(entries, "checksum file is empty")

    for expected_digest, name in entries:
        artifact = dist / Path(name).name
        check(artifact.is_file(), f"checksum references missing artifact: {name}")
        digest = hashlib.sha256(artifact.read_bytes()).hexdigest()
        check(digest == expected_digest, f"checksum mismatch for {artifact.name}")


def archive_members(path: Path) -> set[str]:
    if path.name.endswith(".zip"):
        with zipfile.ZipFile(path) as archive:
            return set(archive.namelist())
    with tarfile.open(path, mode="r:gz") as archive:
        return {member.name for member in archive.getmembers()}


def verify_archives(dist: Path) -> None:
    archives = sorted([*dist.glob("*.tar.gz"), *dist.glob("*.zip")])
    check(archives, f"no release archives found in {dist}")
    for archive in archives:
        names = archive_members(archive)
        missing = {
            required
            for required in REQUIRED_ARCHIVE_FILES
            if not any(name == required or name.endswith(f"/{required}") for name in names)
        }
        check(not missing, f"{archive.name} is missing {sorted(missing)}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-version", required=True)
    parser.add_argument("--cli", type=Path)
    parser.add_argument("--python-root", type=Path)
    parser.add_argument("--wheel-dir", type=Path)
    parser.add_argument("--dist", type=Path)
    parser.add_argument("--checksums", type=Path)
    args = parser.parse_args()

    check(any((args.cli, args.python_root, args.dist, args.checksums)), "at least one verification target is required")
    if args.cli:
        check(args.cli.is_file(), f"CLI binary not found: {args.cli}")
        verify_cli(args.cli, args.expected_version)
        print(f"verified CLI version {args.expected_version}")
    if args.python_root:
        verify_python(args.python_root, args.wheel_dir, args.expected_version)
        print(f"verified Python package version {args.expected_version}")
    if args.checksums:
        check(args.dist is not None, "--dist is required with --checksums")
        verify_checksums(args.dist, args.checksums)
        verify_archives(args.dist)
        print(f"verified {len([*args.dist.glob('*.tar.gz'), *args.dist.glob('*.zip')])} archives and checksums")
    elif args.dist:
        verify_archives(args.dist)
        print(f"verified release archives in {args.dist}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
