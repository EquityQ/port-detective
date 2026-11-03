"""Verify release versions, checksums, and archive contents.

The strict ``--require-platform-manifest`` gate additionally proves that a
GoReleaser output directory holds exactly the six artifacts declared by
``.goreleaser.yaml``: every documented archive present, no extra archive, one
SHA-256 entry per archive (and no checksum for a missing archive), and the
platform executable inside every archive.
"""

from __future__ import annotations

import argparse
import hashlib
import re
import stat
import subprocess
import tarfile
import zipfile
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path, PurePosixPath


REQUIRED_ARCHIVE_FILES = {"README.md", "LICENSE"}

# Authoritative release matrix, mirrored from .goreleaser.yaml: builds over
# goos [linux, darwin, windows] x goarch [amd64, arm64], tar.gz archives with a
# zip override for windows, and one `port-detective` executable per archive.
GOOS_TARGETS = ("linux", "darwin", "windows")
GOARCH_TARGETS = ("amd64", "arm64")
ARCHIVE_SUFFIX_BY_GOOS = {"linux": ".tar.gz", "darwin": ".tar.gz", "windows": ".zip"}
EXECUTABLE_NAME_BY_GOOS = {
    "linux": "port-detective",
    "darwin": "port-detective",
    "windows": "port-detective.exe",
}
CHECKSUM_FILE_NAME = "checksums.txt"
VERSION_PATTERN = re.compile(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?")


def check(condition: bool, message: str) -> None:
    if not condition:
        raise SystemExit(message)


@dataclass(frozen=True)
class ReleaseTarget:
    """One build/archive combination in the release matrix."""

    goos: str
    goarch: str
    archive_suffix: str
    executable_name: str

    @property
    def platform(self) -> str:
        return f"{self.goos}/{self.goarch}"


def _release_targets() -> tuple[ReleaseTarget, ...]:
    targets: list[ReleaseTarget] = []
    for goos in GOOS_TARGETS:
        for goarch in GOARCH_TARGETS:
            check(
                goos in ARCHIVE_SUFFIX_BY_GOOS and goos in EXECUTABLE_NAME_BY_GOOS,
                f"release matrix has no archive convention for {goos}",
            )
            targets.append(
                ReleaseTarget(
                    goos=goos,
                    goarch=goarch,
                    archive_suffix=ARCHIVE_SUFFIX_BY_GOOS[goos],
                    executable_name=EXECUTABLE_NAME_BY_GOOS[goos],
                )
            )
    return tuple(targets)


RELEASE_TARGETS: tuple[ReleaseTarget, ...] = _release_targets()


def expected_archive_name(version: str, target: ReleaseTarget) -> str:
    return f"port-detective_{version}_{target.goos}_{target.goarch}{target.archive_suffix}"


def expected_archives(version: str) -> dict[str, ReleaseTarget]:
    """Map every archive filename the release must contain to its build target."""
    check(bool(VERSION_PATTERN.fullmatch(version)), f"invalid release version: {version}")
    return {expected_archive_name(version, target): target for target in RELEASE_TARGETS}


@dataclass(frozen=True)
class ArchiveEntry:
    """One member of a release archive, classified so directories are never mistaken for files."""

    name: str
    is_file: bool
    is_directory: bool

    @property
    def parts(self) -> tuple[str, ...]:
        return PurePosixPath(self.name.replace("\\", "/")).parts


def archive_entries(path: Path) -> list[ArchiveEntry]:
    try:
        if path.name.endswith(".zip"):
            with zipfile.ZipFile(path) as archive:
                return [_zip_entry(info) for info in archive.infolist()]
        with tarfile.open(path, mode="r:gz") as archive:
            return [_tar_entry(member) for member in archive.getmembers()]
    except (tarfile.TarError, zipfile.BadZipFile, OSError) as error:
        raise SystemExit(f"unreadable release archive {path.name}: {error}") from error


def _zip_entry(info: zipfile.ZipInfo) -> ArchiveEntry:
    is_directory = info.is_dir()
    is_symlink = stat.S_ISLNK(info.external_attr >> 16)
    return ArchiveEntry(
        name=info.filename,
        is_file=not is_directory and not is_symlink,
        is_directory=is_directory,
    )


def _tar_entry(member: tarfile.TarInfo) -> ArchiveEntry:
    return ArchiveEntry(
        name=member.name,
        is_file=member.isfile(),
        is_directory=member.isdir(),
    )


def has_regular_file(
    entries: Sequence[ArchiveEntry], filename: str, max_depth: int | None = None
) -> bool:
    """True when ``filename`` is a regular file member at a depth of at most ``max_depth``."""
    for entry in entries:
        if not entry.is_file or not entry.parts or entry.parts[-1] != filename:
            continue
        if max_depth is None or len(entry.parts) <= max_depth:
            return True
    return False


def has_directory(entries: Sequence[ArchiveEntry], filename: str) -> bool:
    return any(
        entry.is_directory and entry.parts and entry.parts[-1] == filename
        for entry in entries
    )


def discover_archives(dist: Path) -> list[Path]:
    check(dist.is_dir(), f"release directory not found: {dist}")
    return sorted([*dist.glob("*.tar.gz"), *dist.glob("*.zip")])


def verify_archive_contents(path: Path, target: ReleaseTarget) -> None:
    """Require the platform executable plus README/LICENSE inside one release archive."""
    check(path.is_file(), f"release archive not found: {path}")
    entries = archive_entries(path)
    for required in sorted(REQUIRED_ARCHIVE_FILES):
        check(has_regular_file(entries, required), f"{path.name} is missing {required}")

    executable = target.executable_name
    hint = (
        " (found a same-named directory instead of a regular file)"
        if has_directory(entries, executable)
        else ""
    )
    check(
        has_regular_file(entries, executable, max_depth=2),
        f"{path.name} is missing the {target.platform} executable {executable!r}"
        f" at the archive root or under a single directory prefix{hint}",
    )


def verify_release_manifest(dist: Path, version: str, checksums: Path) -> None:
    """Fail unless ``dist`` holds exactly the documented, checksummed, executable-bearing matrix."""
    expected = expected_archives(version)
    present = {archive.name: archive for archive in discover_archives(dist)}

    missing = [name for name in expected if name not in present]
    check(
        not missing,
        "missing platform archive(s): "
        + ", ".join(f"{name} ({expected[name].platform})" for name in missing),
    )
    unexpected = sorted(name for name in present if name not in expected)
    check(not unexpected, "unexpected release archive(s): " + ", ".join(unexpected))

    entries = parse_checksums(checksums)
    unchecksummed = [name for name in expected if name not in entries]
    check(
        not unchecksummed,
        "release archive(s) with no checksum entry: " + ", ".join(unchecksummed),
    )
    unmatched = sorted(name for name in entries if name not in expected)
    check(not unmatched, "checksum entry with no matching release archive: " + ", ".join(unmatched))
    verify_checksum_digests(dist, entries)

    for name, target in expected.items():
        verify_archive_contents(present[name], target)


def verify_cli(path: Path, expected: str) -> None:
    # Resolve the executable before invoking it. On Unix, subprocess does not
    # search the current directory when a relative path is normalized to a bare
    # filename such as `port-detective`.
    executable = path.resolve()
    result = subprocess.run(
        [str(executable), "--version"],
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


def verify_python(root: Path, distribution_dir: Path | None, expected: str) -> None:
    pyproject_version = read_declared_version(
        root / "pyproject.toml", r'^version\s*=\s*["\']([^"\']+)["\']$'
    )
    init_version = read_declared_version(
        root / "port_detective" / "__init__.py",
        r'^__version__\s*=\s*["\']([^"\']+)["\']$',
    )
    check(pyproject_version == expected, f"pyproject version is {pyproject_version}, expected {expected}")
    check(init_version == expected, f"__version__ is {init_version}, expected {expected}")

    if distribution_dir is None:
        return
    wheels = sorted(distribution_dir.glob("*.whl"))
    check(len(wheels) == 1, f"expected one release wheel in {distribution_dir}, found {len(wheels)}")
    with zipfile.ZipFile(wheels[0]) as archive:
        metadata_names = [name for name in archive.namelist() if name.endswith(".dist-info/METADATA")]
        check(len(metadata_names) == 1, f"wheel metadata is ambiguous: {metadata_names}")
        metadata = archive.read(metadata_names[0]).decode("utf-8")
    wheel_version = read_declared_version_from_text(metadata, r"^Version:[ \t]*(\S+)[ \t]*$")
    check(wheel_version == expected, f"wheel version is {wheel_version}, expected {expected}")

    sdists = sorted(distribution_dir.glob("*.tar.gz"))
    check(len(sdists) == 1, f"expected one release sdist in {distribution_dir}, found {len(sdists)}")
    with tarfile.open(sdists[0], mode="r:gz") as archive:
        metadata_members = [
            member
            for member in archive.getmembers()
            if Path(member.name).name == "PKG-INFO"
            and len(Path(member.name).parts) == 2
        ]
        check(len(metadata_members) == 1, f"sdist metadata is ambiguous: {metadata_members}")
        metadata_file = archive.extractfile(metadata_members[0])
        check(metadata_file is not None, f"could not read sdist metadata: {sdists[0]}")
        sdist_metadata = metadata_file.read().decode("utf-8")
    sdist_version = read_declared_version_from_text(sdist_metadata, r"^Version:[ \t]*(\S+)[ \t]*$")
    check(sdist_version == expected, f"sdist version is {sdist_version}, expected {expected}")


def read_declared_version_from_text(text: str, pattern: str) -> str:
    normalized = text.replace("\r\n", "\n").replace("\r", "\n")
    match = re.search(pattern, normalized, flags=re.MULTILINE)
    check(match is not None, "version declaration not found in wheel metadata")
    return match.group(1)


def parse_checksums(checksums: Path) -> dict[str, str]:
    """Parse a GoReleaser checksum file into an ordered {artifact name: sha256} map."""
    check(checksums.is_file(), f"checksum file not found: {checksums}")
    entries: dict[str, str] = {}
    for line in checksums.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line:
            continue
        match = re.fullmatch(r"([0-9a-fA-F]{64})\s+\*?(.+)", line)
        check(match is not None, f"invalid checksum line: {line!r}")
        name = Path(match.group(2)).name
        check(name not in entries, f"duplicate checksum entry: {name}")
        entries[name] = match.group(1).lower()
    check(entries, "checksum file is empty")
    return entries


def verify_checksum_digests(dist: Path, entries: Mapping[str, str]) -> None:
    for name, expected_digest in entries.items():
        artifact = dist / name
        check(artifact.is_file(), f"checksum references missing artifact: {name}")
        digest = hashlib.sha256(artifact.read_bytes()).hexdigest()
        check(digest == expected_digest, f"checksum mismatch for {artifact.name}")


def verify_checksums(dist: Path, checksums: Path) -> dict[str, str]:
    entries = parse_checksums(checksums)
    verify_checksum_digests(dist, entries)
    return entries


def verify_archives(dist: Path) -> None:
    archives = discover_archives(dist)
    check(archives, f"no release archives found in {dist}")
    for archive in archives:
        entries = archive_entries(archive)
        missing = [
            required
            for required in sorted(REQUIRED_ARCHIVE_FILES)
            if not has_regular_file(entries, required)
        ]
        check(not missing, f"{archive.name} is missing {missing}")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-version", required=True)
    parser.add_argument("--cli", type=Path)
    parser.add_argument("--python-root", type=Path)
    parser.add_argument("--wheel-dir", type=Path, help="Python wheel and sdist directory")
    parser.add_argument("--dist", type=Path)
    parser.add_argument("--checksums", type=Path)
    parser.add_argument(
        "--require-platform-manifest",
        action="store_true",
        help=(
            "require exactly the six documented platform archives, a checksum entry for every "
            "archive and no checksum for a missing archive, and the platform executable inside "
            "every archive"
        ),
    )
    args = parser.parse_args()

    check(any((args.cli, args.python_root, args.dist, args.checksums)), "at least one verification target is required")
    if args.require_platform_manifest:
        check(args.dist is not None, "--require-platform-manifest requires --dist")
        check(args.checksums is not None, "--require-platform-manifest requires --checksums")
    if args.cli:
        check(args.cli.is_file(), f"CLI binary not found: {args.cli}")
        verify_cli(args.cli, args.expected_version)
        print(f"verified CLI version {args.expected_version}")
    if args.python_root:
        verify_python(args.python_root, args.wheel_dir, args.expected_version)
        print(f"verified Python package version {args.expected_version}")
    if args.checksums:
        check(args.dist is not None, "--dist is required with --checksums")
        if args.require_platform_manifest:
            verify_release_manifest(args.dist, args.expected_version, args.checksums)
            print(
                f"verified the exact {len(RELEASE_TARGETS)}-platform release manifest "
                f"for {args.expected_version}"
            )
        else:
            verify_checksums(args.dist, args.checksums)
            verify_archives(args.dist)
            print(f"verified {len(discover_archives(args.dist))} archives and checksums")
    elif args.dist:
        verify_archives(args.dist)
        print(f"verified release archives in {args.dist}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
