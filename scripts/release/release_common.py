"""Shared release target matrix and safe archive helpers."""

from __future__ import annotations

import re
import tarfile
import zipfile
from dataclasses import dataclass
from pathlib import Path, PurePosixPath


VERSION_PATTERN = re.compile(r"\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?")


@dataclass(frozen=True)
class ReleaseTarget:
    """One build/archive/npm combination in the six-platform release matrix."""

    node_platform: str
    node_arch: str
    goos: str
    goarch: str
    archive_suffix: str
    executable_name: str

    @property
    def platform(self) -> str:
        return f"{self.goos}/{self.goarch}"

    @property
    def npm_package_name(self) -> str:
        return f"port-detective-cli-{self.node_platform}-{self.node_arch}"


# Authoritative release matrix mirrored from .goreleaser.yaml.
RELEASE_TARGETS: tuple[ReleaseTarget, ...] = (
    ReleaseTarget("linux", "x64", "linux", "amd64", ".tar.gz", "port-detective"),
    ReleaseTarget("linux", "arm64", "linux", "arm64", ".tar.gz", "port-detective"),
    ReleaseTarget("darwin", "x64", "darwin", "amd64", ".tar.gz", "port-detective"),
    ReleaseTarget("darwin", "arm64", "darwin", "arm64", ".tar.gz", "port-detective"),
    ReleaseTarget("win32", "x64", "windows", "amd64", ".zip", "port-detective.exe"),
    ReleaseTarget("win32", "arm64", "windows", "arm64", ".zip", "port-detective.exe"),
)
EXECUTABLE_NAME_BY_GOOS: dict[str, str] = {
    target.goos: target.executable_name for target in RELEASE_TARGETS
}


def safe_archive_member(name: str) -> PurePosixPath:
    path = PurePosixPath(name.replace("\\", "/"))
    if path.is_absolute() or ".." in path.parts:
        raise SystemExit(f"unsafe archive member path: {name}")
    return path


def extract_binary_from_archive(archive: Path, binary_name: str) -> bytes:
    if archive.suffix == ".zip":
        with zipfile.ZipFile(archive) as source:
            for member in source.infolist():
                path = safe_archive_member(member.filename)
                if not member.is_dir() and path.name == binary_name:
                    return source.read(member)
    else:
        with tarfile.open(archive, "r:gz") as source:
            for member in source.getmembers():
                path = safe_archive_member(member.name)
                if member.isfile() and path.name == binary_name:
                    handle = source.extractfile(member)
                    if handle is not None:
                        return handle.read()
    raise SystemExit(f"{binary_name} not found in {archive}")
