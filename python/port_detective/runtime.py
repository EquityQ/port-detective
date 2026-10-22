"""Resolve and cache the platform CLI used by the Python SDK."""

from __future__ import annotations

import hashlib
import importlib.metadata
import io
import os
import platform
import re
import stat
import sys
import tarfile
import tempfile
import urllib.error
import urllib.request
import zipfile
from pathlib import Path, PurePosixPath
from threading import Lock
from typing import Optional, Tuple


REPOSITORY = "EquityQ/port-detective"
RELEASE_BASE_URL = f"https://github.com/{REPOSITORY}/releases/download"
DOWNLOAD_TIMEOUT = 30.0
MAX_DOWNLOAD_BYTES = 100 * 1024 * 1024
MAX_BINARY_BYTES = 50 * 1024 * 1024
_DOWNLOAD_LOCK = Lock()


class RuntimeDownloadError(RuntimeError):
    """Raised when a matching release binary cannot be downloaded safely."""


def auto_download_enabled() -> bool:
    """Return whether automatic CLI downloads are enabled."""
    value = os.getenv("PORT_DETECTIVE_AUTO_DOWNLOAD", "1").strip().lower()
    return value not in {"0", "false", "no", "off"}


def _release_version() -> str:
    version_file = Path(__file__).with_name("__init__.py")
    try:
        source = version_file.read_text(encoding="utf-8")
        source_match = re.search(r'^__version__\s*=\s*["\']([^"\']+)', source, re.MULTILINE)
    except OSError:
        source_match = None
    if source_match:
        value = source_match.group(1)
    else:
        value = ""
    if not value:
        try:
            value = importlib.metadata.version("port-detective")
        except importlib.metadata.PackageNotFoundError:
            value = "1.1.0.dev0"
    match = re.match(r"^(\d+\.\d+\.\d+)", value)
    if not match:
        raise RuntimeDownloadError(
            f"Cannot determine a stable port-detective release from version {value!r}"
        )
    return match.group(1)


def platform_target() -> Tuple[str, str, str]:
    """Return Go release OS, architecture, and executable filename."""
    system = platform.system().lower()
    machine = platform.machine().lower()
    systems = {"windows": "windows", "darwin": "darwin", "linux": "linux"}
    goos = systems.get(system)
    architectures = {
        "amd64": "amd64",
        "x86_64": "amd64",
        "x64": "amd64",
        "arm64": "arm64",
        "aarch64": "arm64",
    }
    goarch = architectures.get(machine)
    if not goos or not goarch:
        raise RuntimeDownloadError(
            f"Automatic CLI download is unsupported on {system or 'unknown'}/{machine or 'unknown'}"
        )
    executable = "port-detective.exe" if goos == "windows" else "port-detective"
    return goos, goarch, executable


def cache_directory() -> Path:
    """Return the user-writable cache directory for downloaded CLIs."""
    configured = os.getenv("PORT_DETECTIVE_CACHE_DIR")
    if configured:
        return Path(configured).expanduser()
    if os.name == "nt":
        root = os.getenv("LOCALAPPDATA") or os.getenv("TEMP") or str(Path.home())
        return Path(root) / "port-detective" / "bin"
    if platform.system().lower() == "darwin":
        return Path.home() / "Library" / "Caches" / "port-detective" / "bin"
    root = os.getenv("XDG_CACHE_HOME") or (Path.home() / ".cache")
    return Path(root) / "port-detective" / "bin"


def _download(url: str) -> bytes:
    request = urllib.request.Request(
        url,
        headers={"User-Agent": "port-detective-python-sdk"},
    )
    try:
        with urllib.request.urlopen(request, timeout=DOWNLOAD_TIMEOUT) as response:
            data = response.read(MAX_DOWNLOAD_BYTES + 1)
    except (OSError, urllib.error.URLError) as exc:
        raise RuntimeDownloadError(f"Failed to download {url}: {exc}") from exc
    if len(data) > MAX_DOWNLOAD_BYTES:
        raise RuntimeDownloadError(f"Downloaded release asset is larger than {MAX_DOWNLOAD_BYTES} bytes")
    return data


def _checksum(checksums: bytes, asset_name: str) -> str:
    for line in checksums.decode("utf-8", errors="replace").splitlines():
        fields = line.strip().split()
        if len(fields) >= 2 and Path(fields[-1]).name == asset_name:
            digest = fields[0].lower()
            if re.fullmatch(r"[0-9a-f]{64}", digest):
                return digest
    raise RuntimeDownloadError(f"Release checksums do not contain {asset_name}")


def _safe_member(name: str) -> PurePosixPath:
    path = PurePosixPath(name.replace("\\", "/"))
    if path.is_absolute() or ".." in path.parts:
        raise RuntimeDownloadError(f"Unsafe release archive member: {name}")
    return path


def _usable_target(path: Path) -> bool:
    try:
        return path.is_file() and path.stat().st_size > 0
    except OSError:
        return False


def _cached_target_valid(path: Path) -> bool:
    if not _usable_target(path):
        return False
    checksum_path = path.with_name(path.name + ".sha256")
    try:
        expected = checksum_path.read_text(encoding="ascii").strip().lower()
        if not re.fullmatch(r"[0-9a-f]{64}", expected):
            return False
        actual = hashlib.sha256(path.read_bytes()).hexdigest()
    except (OSError, UnicodeError):
        return False
    return actual == expected


def _extract_binary(archive: bytes, archive_name: str, executable: str) -> bytes:
    try:
        if archive_name.endswith(".zip"):
            source = zipfile.ZipFile(io.BytesIO(archive))
            with source:
                for member in source.infolist():
                    path = _safe_member(member.filename)
                    if not member.is_dir() and path.name == executable:
                        if member.file_size > MAX_BINARY_BYTES:
                            raise RuntimeDownloadError(
                                f"Extracted CLI is larger than {MAX_BINARY_BYTES} bytes"
                            )
                        return source.read(member)
        else:
            source = tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz")
            with source:
                for member in source.getmembers():
                    path = _safe_member(member.name)
                    if member.isfile() and path.name == executable:
                        if member.size > MAX_BINARY_BYTES:
                            raise RuntimeDownloadError(
                                f"Extracted CLI is larger than {MAX_BINARY_BYTES} bytes"
                            )
                        handle = source.extractfile(member)
                        if handle is not None:
                            return handle.read()
    except (OSError, tarfile.TarError, zipfile.BadZipFile) as exc:
        raise RuntimeDownloadError(f"Invalid release archive {archive_name}: {exc}") from exc
    raise RuntimeDownloadError(f"Release archive does not contain {executable}")


def download_cli(*, version: Optional[str] = None, destination: Optional[Path] = None) -> str:
    """Download, verify, extract, and cache the matching release CLI."""
    goos, goarch, executable = platform_target()
    release = version or _release_version()
    if not re.fullmatch(r"\d+\.\d+\.\d+", release):
        raise RuntimeDownloadError(f"Invalid release version: {release}")
    archive_suffix = "zip" if goos == "windows" else "tar.gz"
    archive_name = f"port-detective_{release}_{goos}_{goarch}.{archive_suffix}"
    cache_root = Path(destination) if destination else cache_directory()
    target_dir = cache_root / release / f"{goos}-{goarch}"
    target = target_dir / executable
    if _cached_target_valid(target):
        return str(target)

    with _DOWNLOAD_LOCK:
        if _cached_target_valid(target):
            return str(target)
        archive_url = f"{RELEASE_BASE_URL}/v{release}/{archive_name}"
        checksum_url = f"{RELEASE_BASE_URL}/v{release}/checksums.txt"
        print(f"port-detective: downloading verified CLI v{release} for {goos}/{goarch}...", file=sys.stderr)
        archive = _download(archive_url)
        expected = _checksum(_download(checksum_url), archive_name)
        actual = hashlib.sha256(archive).hexdigest()
        if actual != expected:
            raise RuntimeDownloadError(
                f"Checksum mismatch for {archive_name}: expected {expected}, got {actual}"
            )
        binary = _extract_binary(archive, archive_name, executable)
        if not binary or len(binary) > MAX_BINARY_BYTES:
            raise RuntimeDownloadError("Release archive contains an empty or oversized CLI binary")
        temporary_path: Optional[Path] = None
        try:
            target_dir.mkdir(parents=True, exist_ok=True)
            with tempfile.NamedTemporaryFile(
                mode="wb", prefix=f".{executable}.", dir=target_dir, delete=False
            ) as temporary:
                temporary.write(binary)
                temporary_path = Path(temporary.name)
            if goos != "windows":
                temporary_path.chmod(stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR)
            os.replace(temporary_path, target)
            binary_digest = hashlib.sha256(binary).hexdigest()
            target.with_name(target.name + ".sha256").write_text(binary_digest + "\n", encoding="ascii")
        except OSError as exc:
            raise RuntimeDownloadError(f"Cannot cache the downloaded CLI at {target}: {exc}") from exc
        finally:
            if temporary_path is not None:
                temporary_path.unlink(missing_ok=True)
    return str(target)
