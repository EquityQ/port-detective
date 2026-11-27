"""
port_detective.core
~~~~~~~~~~~~~~~~~~~

Subprocess client wrapper for executing port-detective CLI in --json mode.
"""

import asyncio
import json
import math
import os
import re
import shutil
import subprocess
import time
from pathlib import Path
from typing import Optional, List, Dict, Any, Tuple

from .errors import (
    PortDetectiveError,
    PortDetectiveNotFoundError,
    PortDetectiveVersionError,
    PortDetectiveExecutionError,
)
from .models import PortDiagnostic, KillResult, CleanResult, FreePortResult, DoctorReport
from .runtime import RuntimeDownloadError, _release_version, auto_download_enabled, download_cli

DEFAULT_CLI_TIMEOUT = 30.0
_VALIDATED_BINARIES: Dict[str, Tuple[int, ...]] = {}
_VALID_PROTOCOLS = {"tcp", "udp"}
_VALID_PRESETS = {"web", "python", "database", "project"}
_VALID_WAIT_STATES = {"open", "free"}


def _validate_port(port: int) -> None:
    if isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535:
        raise ValueError(f"port must be an integer between 1 and 65535, got {port!r}")


def _validate_protocol(protocol: str) -> str:
    if not isinstance(protocol, str) or protocol.strip().lower() not in _VALID_PROTOCOLS:
        raise ValueError(f"protocol must be 'tcp' or 'udp', got {protocol!r}")
    return protocol.strip().lower()


def _validate_preview_id(preview_id: Optional[str]) -> Optional[str]:
    if preview_id is None:
        return None
    if not isinstance(preview_id, str) or not preview_id.strip():
        raise ValueError("preview_id must be a non-empty string or None")
    return preview_id


def _validate_preset(preset: Optional[str]) -> Optional[str]:
    if preset is None:
        return None
    if not isinstance(preset, str) or preset not in _VALID_PRESETS:
        raise ValueError(f"preset must be one of {sorted(_VALID_PRESETS)}, got {preset!r}")
    return preset


def _format_duration_ms(seconds: float, label: str) -> str:
    if isinstance(seconds, bool) or not isinstance(seconds, (int, float)) or not math.isfinite(seconds) or seconds <= 0:
        raise ValueError(f"{label} must be a positive number of seconds, got {seconds!r}")
    ms = max(1, int(round(seconds * 1000)))
    return f"{ms}ms"


def _normalize_ports(ports: Optional[List[int]]) -> Optional[List[int]]:
    """Return the explicit port selection, or None to mean "use the documented default".

    An omitted (None) selection means the caller did not choose a scope, so the CLI
    default applies. An explicitly empty list means "select nothing" and is rejected
    here, because forwarding it as an omitted argument would silently widen the scope
    to the whole default set or to every project listener.
    """
    if ports is None:
        return None
    if isinstance(ports, (str, bytes)):
        raise ValueError(f"ports must be an iterable of integers or None, got {ports!r}")
    try:
        selected = list(ports)
    except TypeError:
        raise ValueError(
            f"ports must be an iterable of integers or None, got {ports!r}"
        ) from None
    if not selected:
        raise ValueError(
            "ports must not be empty; omit it or pass None to use the default selection"
        )
    for port in selected:
        _validate_port(port)
    return selected


def _validate_timeout(timeout: Optional[float]) -> None:
    if timeout is not None and (
        isinstance(timeout, bool)
        or not isinstance(timeout, (int, float))
        or not math.isfinite(timeout)
        or timeout <= 0
    ):
        raise ValueError(f"timeout must be positive or None, got {timeout!r}")


def _validate_cwd(cwd: Optional[str]) -> None:
    if cwd is not None and not Path(cwd).is_dir():
        raise PortDetectiveExecutionError(
            f"Working directory does not exist or is not a directory: {cwd}",
            returncode=-1,
        )


def _decode_output(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return str(value)


async def _terminate_async_process(proc: Any) -> tuple[str, str]:
    """Terminate and reap a CLI child so timeout/cancellation cannot leak it."""
    if proc.returncode is None:
        proc.kill()
    stdout_bytes, stderr_bytes = await proc.communicate()
    return _decode_output(stdout_bytes).strip(), _decode_output(stderr_bytes).strip()


def _binary_identity(binary: str) -> Optional[tuple]:
    try:
        info = Path(binary).stat()
        return (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
    except OSError:
        return None


def _version_cached(binary: str) -> bool:
    identity = _binary_identity(binary)
    return identity is not None and _VALIDATED_BINARIES.get(binary) == identity


def _remember_binary(binary: str) -> None:
    identity = _binary_identity(binary)
    if identity is not None:
        _VALIDATED_BINARIES[binary] = identity


def _accept_cli_version(binary: str, output: str, identity: Optional[tuple]) -> None:
    match = re.fullmatch(
        r"(?:port-detective(?:\.exe)? version )?v?(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?",
        output.strip(),
    )
    expected = _release_version()
    if match is None or tuple(match.groups()[:2]) != tuple(expected.split(".")[:2]):
        raise PortDetectiveVersionError(
            f"Incompatible port-detective CLI at {binary}: {output.strip()!r}. "
            f"This SDK requires CLI {'.'.join(expected.split('.')[:2])}.x "
            "(including versioned development builds). Upgrade the CLI or set "
            "PORT_DETECTIVE_BIN to a compatible binary; run "
            "'python -m port_detective install' to cache the matching release."
        )
    if identity is not None and _binary_identity(binary) != identity:
        raise PortDetectiveVersionError(f"CLI changed during its version check: {binary}; retry the call")
    _remember_binary(binary)


def _validate_binary_sync(binary: str, timeout: Optional[float]) -> str:
    if _version_cached(binary):
        return binary
    identity = _binary_identity(binary)
    try:
        result = subprocess.run(
            [binary, "--version"], capture_output=True, text=True, encoding="utf-8",
            check=False, timeout=timeout,
        )
    except subprocess.TimeoutExpired as exc:
        raise PortDetectiveExecutionError(
            f"CLI version check timed out after {timeout} seconds: {binary}", -1,
            _decode_output(exc.stderr), _decode_output(exc.stdout),
        ) from exc
    except OSError as exc:
        raise PortDetectiveExecutionError(f"Cannot check CLI version at {binary}: {exc}", exc.errno or -1) from exc
    if result.returncode != 0:
        raise PortDetectiveExecutionError(
            f"CLI version check failed at {binary}: {result.stderr.strip() or result.returncode}",
            result.returncode, result.stderr, result.stdout,
        )
    _accept_cli_version(binary, result.stdout, identity)
    return binary


def find_binary(
    explicit_path: Optional[str] = None, *, allow_download: bool = True,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT, validate_version: bool = True,
) -> str:
    """
    Locate the port-detective executable.

    Resolution order:
    1. explicit_path argument if provided.
    2. PORT_DETECTIVE_BIN environment variable.
    3. System PATH via shutil.which("port-detective").
    4. Nearby repository binary locations.
    5. A verified, cached binary downloaded from the matching GitHub release.
    """
    _validate_timeout(timeout)

    def external(path: str) -> str:
        binary = str(Path(path).resolve())
        return _validate_binary_sync(binary, timeout) if validate_version else binary

    if explicit_path:
        path = Path(explicit_path).resolve()
        if path.is_file():
            return external(str(path))
        raise PortDetectiveNotFoundError(f"Specified binary not found: {explicit_path}")

    env_path = os.getenv("PORT_DETECTIVE_BIN")
    if env_path:
        path = Path(env_path).resolve()
        if path.is_file():
            return external(str(path))
        raise PortDetectiveNotFoundError(f"Binary in PORT_DETECTIVE_BIN not found: {env_path}")

    which_path = shutil.which("port-detective")
    if which_path:
        return external(which_path)

    # Search in common repo root / build directories
    candidates = [
        Path.cwd() / "port-detective",
        Path.cwd() / "port-detective.exe",
        Path(__file__).resolve().parent.parent.parent / "port-detective",
        Path(__file__).resolve().parent.parent.parent / "port-detective.exe",
    ]
    for c in candidates:
        if c.is_file():
            return external(str(c))

    if not allow_download or not auto_download_enabled():
        raise PortDetectiveNotFoundError(
            "Could not find 'port-detective' executable. Automatic download is disabled; "
            "install the matching CLI, set PORT_DETECTIVE_BIN, or enable "
            "PORT_DETECTIVE_AUTO_DOWNLOAD."
        )

    try:
        binary = download_cli()
        _remember_binary(binary)  # The downloader verifies the exact SDK release.
        return binary
    except RuntimeDownloadError as exc:
        raise PortDetectiveNotFoundError(
            "Could not find 'port-detective' executable and automatic download failed. "
            "Install the matching CLI, set PORT_DETECTIVE_BIN, or retry with network access: "
            f"{exc}"
        ) from exc


async def _find_binary_async(binary_path: Optional[str]) -> str:
    try:
        binary = find_binary(binary_path, allow_download=False, validate_version=False)
    except PortDetectiveNotFoundError:
        if binary_path or os.getenv("PORT_DETECTIVE_BIN"):
            raise
        # Downloading may continue caching after cancellation; it never runs on the event loop.
        binary = await asyncio.to_thread(find_binary, None, validate_version=False)
    if _version_cached(binary):
        return binary
    identity = _binary_identity(binary)
    proc = None
    try:
        proc = await asyncio.create_subprocess_exec(
            binary, "--version", stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
        )
        output, errors = await proc.communicate()
    except asyncio.CancelledError:
        if proc is not None:
            try:
                await _terminate_async_process(proc)
            except Exception:
                pass  # Preserve cancellation if the child exits during cleanup.
        raise
    except OSError as exc:
        raise PortDetectiveExecutionError(f"Cannot check CLI version at {binary}: {exc}", exc.errno or -1) from exc
    stdout, stderr = _decode_output(output), _decode_output(errors)
    if proc.returncode != 0:
        raise PortDetectiveExecutionError(
            f"CLI version check failed at {binary}: {stderr.strip() or proc.returncode}",
            proc.returncode if proc.returncode is not None else 1, stderr, stdout,
        )
    _accept_cli_version(binary, stdout, identity)
    return binary


def _run_cli_sync(
    args: List[str],
    cwd: Optional[str] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    allow_exit_codes: Tuple[int, ...] = (0,),
) -> Any:
    _validate_timeout(timeout)
    _validate_cwd(cwd)
    deadline = None if timeout is None else time.monotonic() + timeout
    bin_path = find_binary(binary_path, timeout=timeout)
    cmd = [bin_path, *args, "--json"]
    remaining = None if deadline is None else deadline - time.monotonic()
    if remaining is not None and remaining <= 0:
        raise PortDetectiveExecutionError(f"port-detective timed out after {timeout} seconds", -1)

    try:
        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            cwd=cwd,
            check=False,
            timeout=remaining,
        )
    except subprocess.TimeoutExpired as exc:
        stdout = _decode_output(exc.stdout).strip()
        stderr = _decode_output(exc.stderr).strip()
        raise PortDetectiveExecutionError(
            f"port-detective timed out after {timeout} seconds",
            returncode=-1,
            stderr=stderr,
            stdout=stdout,
        ) from exc
    except FileNotFoundError as exc:
        raise PortDetectiveNotFoundError(f"Failed to execute {bin_path}: {exc}") from exc
    except OSError as exc:
        raise PortDetectiveExecutionError(
            f"Failed to execute {bin_path}: {exc}",
            returncode=exc.errno or -1,
        ) from exc

    if proc.returncode not in allow_exit_codes:
        err_msg = proc.stderr.strip() or f"CLI returned code {proc.returncode}"
        raise PortDetectiveExecutionError(
            err_msg,
            returncode=proc.returncode,
            stderr=proc.stderr,
            stdout=proc.stdout,
        )

    out = proc.stdout.strip()
    if not out:
        return None

    try:
        return json.loads(out)
    except json.JSONDecodeError as exc:
        raise PortDetectiveExecutionError(
            f"Invalid JSON returned from port-detective: {exc}",
            returncode=proc.returncode,
            stderr=proc.stderr,
            stdout=proc.stdout,
        ) from exc


async def _run_cli_async(
    args: List[str],
    cwd: Optional[str] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    allow_exit_codes: Tuple[int, ...] = (0,),
) -> Any:
    _validate_timeout(timeout)
    _validate_cwd(cwd)
    deadline = None if timeout is None else time.monotonic() + timeout
    proc = None
    bin_path = binary_path or "port-detective"
    try:
        discovery = _find_binary_async(binary_path)
        bin_path = await discovery if timeout is None else await asyncio.wait_for(discovery, timeout)
        cmd = [bin_path, *args, "--json"]
        if deadline is not None and time.monotonic() >= deadline:
            raise asyncio.TimeoutError()
        proc = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=cwd,
        )
        if deadline is None:
            stdout_bytes, stderr_bytes = await proc.communicate()
        else:
            remaining = max(0.0, deadline - time.monotonic())
            stdout_bytes, stderr_bytes = await asyncio.wait_for(proc.communicate(), remaining)
    except asyncio.TimeoutError as exc:
        stdout_str, stderr_str = await _terminate_async_process(proc) if proc is not None else ("", "")
        raise PortDetectiveExecutionError(
            f"port-detective timed out after {timeout} seconds",
            returncode=-1,
            stderr=stderr_str,
            stdout=stdout_str,
        ) from exc
    except asyncio.CancelledError:
        if proc is not None:
            try:
                await _terminate_async_process(proc)
            except Exception:
                # Preserve task cancellation even if child cleanup itself fails.
                pass
        raise
    except FileNotFoundError as exc:
        raise PortDetectiveNotFoundError(f"Failed to execute {bin_path}: {exc}") from exc
    except OSError as exc:
        raise PortDetectiveExecutionError(
            f"Failed to execute {bin_path}: {exc}",
            returncode=exc.errno or -1,
        ) from exc

    stdout_str = stdout_bytes.decode("utf-8", errors="replace").strip()
    stderr_str = stderr_bytes.decode("utf-8", errors="replace").strip()

    if proc.returncode not in allow_exit_codes:
        err_msg = stderr_str or f"CLI returned code {proc.returncode}"
        raise PortDetectiveExecutionError(
            err_msg,
            returncode=proc.returncode or 1,
            stderr=stderr_str,
            stdout=stdout_str,
        )

    if not stdout_str:
        return None

    try:
        return json.loads(stdout_str)
    except json.JSONDecodeError as exc:
        raise PortDetectiveExecutionError(
            f"Invalid JSON returned from port-detective: {exc}",
            returncode=proc.returncode or 0,
            stderr=stderr_str,
            stdout=stdout_str,
        ) from exc


# ============================ Command Helpers ============================


def _check_args(port: int, protocol: str) -> List[str]:
    _validate_port(port)
    proto = _validate_protocol(protocol)
    args = ["check", str(port)]
    if proto != "tcp":
        args.extend(["--protocol", proto])
    return args


def _scan_args(
    ports: Optional[List[int]],
    protocol: str = "tcp",
    preset: Optional[str] = None,
    project_path: Optional[str] = None,
) -> List[str]:
    proto = _validate_protocol(protocol)
    checked_preset = _validate_preset(preset)
    selected = _normalize_ports(ports)
    if selected is not None and checked_preset is not None:
        raise ValueError("ports and preset cannot be used together")
    if project_path is not None and checked_preset != "project":
        raise ValueError("project_path requires preset='project'")
    args = ["scan"]
    if selected is not None:
        args.extend(["--ports", ",".join(str(p) for p in selected)])
    if checked_preset is not None:
        args.extend(["--preset", checked_preset])
    if project_path is not None:
        if not isinstance(project_path, str) or not project_path.strip():
            raise ValueError("project_path must be a non-empty directory path")
        args.extend(["--project", project_path])
    if proto != "tcp":
        args.extend(["--protocol", proto])
    return args


def _free_args(
    port: int,
    dry_run: bool,
    preview_id: Optional[str] = None,
    protocol: str = "tcp",
) -> List[str]:
    _validate_port(port)
    proto = _validate_protocol(protocol)
    token = _validate_preview_id(preview_id)
    args = ["kill", str(port), "-y"]
    if dry_run:
        args.append("--dry-run")
    if proto != "tcp":
        args.extend(["--protocol", proto])
    if token is not None:
        args.extend(["--preview-id", token])
    return args


def _clean_args(
    dry_run: bool,
    ports: Optional[List[int]],
    preview_id: Optional[str] = None,
    *,
    project_path: Optional[str] = None,
    envelope: bool = False,
) -> List[str]:
    token = _validate_preview_id(preview_id)
    args = ["clean", "-y"]
    if dry_run:
        args.append("--dry-run")
    if project_path is not None:
        if not isinstance(project_path, str) or not project_path.strip():
            raise ValueError("path must be a non-empty directory path")
        args.extend(["--project", project_path])
    selected = _normalize_ports(ports)
    if selected is not None:
        args.extend(["--ports", ",".join(map(str, selected))])
    if token is not None:
        args.extend(["--preview-id", token])
    if envelope:
        args.append("--envelope")
    return args


def _wait_args(port: int, state: str, wait_timeout: float, interval: float) -> List[str]:
    _validate_port(port)
    if not isinstance(state, str) or state not in _VALID_WAIT_STATES:
        raise ValueError(f"state must be 'open' or 'free', got {state!r}")
    timeout_arg = _format_duration_ms(wait_timeout, "wait_timeout")
    interval_arg = _format_duration_ms(interval, "interval")
    return ["wait", str(port), "--state", state, "--timeout", timeout_arg, "--interval", interval_arg]


def _find_free_args(port_from: int, port_to: int) -> List[str]:
    _validate_port(port_from)
    _validate_port(port_to)
    if port_from > port_to:
        raise ValueError(f"port_from ({port_from}) must be <= port_to ({port_to})")
    return ["find-free", "--from", str(port_from), "--to", str(port_to)]


def _parse_diagnostic(data: Any, command: str) -> PortDiagnostic:
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for {command}, got {type(data)}", returncode=0)
    return PortDiagnostic.from_dict(data)


def _parse_diagnostics(data: Any, command: str) -> List[PortDiagnostic]:
    if not isinstance(data, list):
        if data is None:
            return []
        raise PortDetectiveExecutionError(f"Expected list response for {command}, got {type(data)}", returncode=0)
    return [PortDiagnostic.from_dict(item) for item in data]


def _parse_kill_result(data: Any) -> KillResult:
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for kill, got {type(data)}", returncode=0)
    return KillResult.from_dict(data)


def _parse_clean_result(data: Any) -> CleanResult:
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for clean, got {type(data)}", returncode=0)
    return CleanResult.from_dict(data)


def _parse_free_port_result(data: Any) -> FreePortResult:
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for find-free, got {type(data)}", returncode=0)
    return FreePortResult.from_dict(data)


def _parse_doctor_report(data: Any) -> DoctorReport:
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for doctor, got {type(data)}", returncode=0)
    return DoctorReport.from_dict(data)


# ============================ Synchronous API ============================


def inspect_port(
    port: int,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    protocol: str = "tcp",
) -> PortDiagnostic:
    """
    Inspect a single TCP or UDP port and retrieve process health and framework fingerprint.
    """
    data = _run_cli_sync(_check_args(port, protocol), binary_path=binary_path, timeout=timeout)
    return _parse_diagnostic(data, "check")


def scan_ports(
    ports: Optional[List[int]] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    protocol: str = "tcp",
    preset: Optional[str] = None,
    project_path: Optional[str] = None,
) -> List[PortDiagnostic]:
    """
    Concurrently scan specified development ports (or default dev ports).

    Omit ``ports`` or pass ``None`` to scan the default development ports. An
    explicitly empty list is rejected with :exc:`ValueError` rather than scanning
    the default set, so an empty selection can never widen the scan scope.
    """
    args = _scan_args(ports, protocol=protocol, preset=preset, project_path=project_path)
    data = _run_cli_sync(args, binary_path=binary_path, timeout=timeout)
    return _parse_diagnostics(data, "scan")


def free_port(
    port: int,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    preview_id: Optional[str] = None,
    protocol: str = "tcp",
) -> KillResult:
    """
    Terminate the process occupying the specified port.
    Set dry_run=False to actually kill the process; pass preview_id from a prior
    dry run to bind execution to the previewed process snapshot.
    """
    data = _run_cli_sync(_free_args(port, dry_run, preview_id, protocol), binary_path=binary_path, timeout=timeout)
    return _parse_kill_result(data)


def clean_project(
    path: Optional[str] = None,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    ports: Optional[List[int]] = None,
    preview_id: Optional[str] = None,
) -> List[PortDiagnostic]:
    """
    Concurrently scan dev ports and process listeners belonging to the project path.
    Set dry_run=False to actually kill the processes.

    Omit ``ports`` or pass ``None`` to keep full project discovery. An explicitly
    empty list is rejected with :exc:`ValueError`; it never means "discover
    everything", so an empty selection can only ever narrow the cleanup scope.
    """
    args = _clean_args(dry_run, ports, preview_id, project_path=path)
    target_cwd = path if path is not None else os.getcwd()
    data = _run_cli_sync(args, cwd=target_cwd, binary_path=binary_path, timeout=timeout)
    return _parse_diagnostics(data, "clean")


def clean_project_result(
    path: Optional[str] = None,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    ports: Optional[List[int]] = None,
    preview_id: Optional[str] = None,
) -> CleanResult:
    """Preview or execute project cleanup and return the structured CleanResult envelope."""
    args = _clean_args(dry_run, ports, preview_id, project_path=path, envelope=True)
    target_cwd = path if path is not None else os.getcwd()
    data = _run_cli_sync(args, cwd=target_cwd, binary_path=binary_path, timeout=timeout)
    return _parse_clean_result(data)


def wait_for_port(
    port: int,
    *,
    state: str = "open",
    wait_timeout: float = 30.0,
    interval: float = 0.25,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = None,
) -> PortDiagnostic:
    """Wait until a TCP port becomes open or free within wait_timeout seconds."""
    args = _wait_args(port, state, wait_timeout, interval)
    cli_timeout = (wait_timeout + 5.0) if timeout is None else timeout
    data = _run_cli_sync(args, binary_path=binary_path, timeout=cli_timeout)
    return _parse_diagnostic(data, "wait")


def find_free_port(
    *,
    port_from: int = 3000,
    port_to: int = 3100,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> FreePortResult:
    """Find a currently bindable loopback TCP port within [port_from, port_to]."""
    args = _find_free_args(port_from, port_to)
    data = _run_cli_sync(args, binary_path=binary_path, timeout=timeout)
    return _parse_free_port_result(data)


def doctor(
    *,
    binary_path: Optional[str] = None,
    cwd: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> DoctorReport:
    """Run CLI self-diagnostics and return a structured DoctorReport (including degraded exit 5)."""
    data = _run_cli_sync(["doctor"], cwd=cwd, binary_path=binary_path, timeout=timeout, allow_exit_codes=(0, 5))
    return _parse_doctor_report(data)


# ============================ Asynchronous API ============================


async def async_inspect_port(
    port: int,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    protocol: str = "tcp",
) -> PortDiagnostic:
    """Asynchronously inspect a single TCP or UDP port."""
    data = await _run_cli_async(_check_args(port, protocol), binary_path=binary_path, timeout=timeout)
    return _parse_diagnostic(data, "check")


async def async_scan_ports(
    ports: Optional[List[int]] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    protocol: str = "tcp",
    preset: Optional[str] = None,
    project_path: Optional[str] = None,
) -> List[PortDiagnostic]:
    """Asynchronously concurrently scan ports.

    Omit ``ports`` or pass ``None`` to scan the default development ports; an
    explicitly empty list raises :exc:`ValueError` instead of scanning the default set.
    """
    args = _scan_args(ports, protocol=protocol, preset=preset, project_path=project_path)
    data = await _run_cli_async(args, binary_path=binary_path, timeout=timeout)
    return _parse_diagnostics(data, "scan")


async def async_free_port(
    port: int,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    preview_id: Optional[str] = None,
    protocol: str = "tcp",
) -> KillResult:
    """Asynchronously terminate the process occupying the specified port."""
    data = await _run_cli_async(_free_args(port, dry_run, preview_id, protocol), binary_path=binary_path, timeout=timeout)
    return _parse_kill_result(data)


async def async_clean_project(
    path: Optional[str] = None,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    ports: Optional[List[int]] = None,
    preview_id: Optional[str] = None,
) -> List[PortDiagnostic]:
    """Asynchronously clean listeners belonging to the project path.

    Omit ``ports`` or pass ``None`` to keep full project discovery; an explicitly
    empty list raises :exc:`ValueError` instead of widening the cleanup scope.
    """
    args = _clean_args(dry_run, ports, preview_id, project_path=path)
    target_cwd = path if path is not None else os.getcwd()
    data = await _run_cli_async(args, cwd=target_cwd, binary_path=binary_path, timeout=timeout)
    return _parse_diagnostics(data, "clean")


async def async_clean_project_result(
    path: Optional[str] = None,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
    *,
    ports: Optional[List[int]] = None,
    preview_id: Optional[str] = None,
) -> CleanResult:
    """Asynchronously preview or execute project cleanup and return a CleanResult envelope."""
    args = _clean_args(dry_run, ports, preview_id, project_path=path, envelope=True)
    target_cwd = path if path is not None else os.getcwd()
    data = await _run_cli_async(args, cwd=target_cwd, binary_path=binary_path, timeout=timeout)
    return _parse_clean_result(data)


async def async_wait_for_port(
    port: int,
    *,
    state: str = "open",
    wait_timeout: float = 30.0,
    interval: float = 0.25,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = None,
) -> PortDiagnostic:
    """Asynchronously wait until a TCP port becomes open or free."""
    args = _wait_args(port, state, wait_timeout, interval)
    cli_timeout = (wait_timeout + 5.0) if timeout is None else timeout
    data = await _run_cli_async(args, binary_path=binary_path, timeout=cli_timeout)
    return _parse_diagnostic(data, "wait")


async def async_find_free_port(
    *,
    port_from: int = 3000,
    port_to: int = 3100,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> FreePortResult:
    """Asynchronously find a bindable loopback TCP port within [port_from, port_to]."""
    args = _find_free_args(port_from, port_to)
    data = await _run_cli_async(args, binary_path=binary_path, timeout=timeout)
    return _parse_free_port_result(data)


async def async_doctor(
    *,
    binary_path: Optional[str] = None,
    cwd: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> DoctorReport:
    """Asynchronously run CLI self-diagnostics and return a DoctorReport."""
    data = await _run_cli_async(["doctor"], cwd=cwd, binary_path=binary_path, timeout=timeout, allow_exit_codes=(0, 5))
    return _parse_doctor_report(data)
