"""
port_detective.core
~~~~~~~~~~~~~~~~~~~

Subprocess client wrapper for executing port-detective CLI in --json mode.
"""

import asyncio
import json
import os
import shutil
import subprocess
from pathlib import Path
from typing import Optional, List, Dict, Any, Union

from .models import PortDiagnostic, KillResult

DEFAULT_CLI_TIMEOUT = 30.0


class PortDetectiveError(Exception):
    """Base exception for port-detective errors."""
    pass


class PortDetectiveNotFoundError(PortDetectiveError):
    """Raised when the port-detective executable cannot be found."""
    pass


class PortDetectiveExecutionError(PortDetectiveError):
    """Raised when port-detective exits with a non-zero exit code."""
    def __init__(self, message: str, returncode: int, stderr: str = "", stdout: str = ""):
        super().__init__(message)
        self.returncode = returncode
        self.stderr = stderr
        self.stdout = stdout


def _validate_port(port: int) -> None:
    if isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535:
        raise ValueError(f"port must be an integer between 1 and 65535, got {port!r}")


def _validate_timeout(timeout: Optional[float]) -> None:
    if timeout is not None and (
        isinstance(timeout, bool)
        or not isinstance(timeout, (int, float))
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


def find_binary(explicit_path: Optional[str] = None) -> str:
    """
    Locate the port-detective executable.

    Resolution order:
    1. explicit_path argument if provided.
    2. PORT_DETECTIVE_BIN environment variable.
    3. System PATH via shutil.which("port-detective").
    4. Nearby repository binary locations.
    """
    if explicit_path:
        path = Path(explicit_path).resolve()
        if path.is_file():
            return str(path)
        raise PortDetectiveNotFoundError(f"Specified binary not found: {explicit_path}")

    env_path = os.getenv("PORT_DETECTIVE_BIN")
    if env_path:
        path = Path(env_path).resolve()
        if path.is_file():
            return str(path)
        raise PortDetectiveNotFoundError(f"Binary in PORT_DETECTIVE_BIN not found: {env_path}")

    which_path = shutil.which("port-detective")
    if which_path:
        return which_path

    # Search in common repo root / build directories
    candidates = [
        Path.cwd() / "port-detective",
        Path.cwd() / "port-detective.exe",
        Path(__file__).resolve().parent.parent.parent / "port-detective",
        Path(__file__).resolve().parent.parent.parent / "port-detective.exe",
    ]
    for c in candidates:
        if c.is_file():
            return str(c)

    raise PortDetectiveNotFoundError(
        "Could not find 'port-detective' executable. "
        "Please ensure it is installed on your PATH or set PORT_DETECTIVE_BIN."
    )


def _run_cli_sync(
    args: List[str],
    cwd: Optional[str] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> Any:
    _validate_timeout(timeout)
    _validate_cwd(cwd)
    bin_path = find_binary(binary_path)
    cmd = [bin_path, *args, "--json"]

    try:
        proc = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            cwd=cwd,
            check=False,
            timeout=timeout,
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

    if proc.returncode != 0:
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
) -> Any:
    _validate_timeout(timeout)
    _validate_cwd(cwd)
    bin_path = find_binary(binary_path)
    cmd = [bin_path, *args, "--json"]

    try:
        proc = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=cwd,
        )
        if timeout is None:
            stdout_bytes, stderr_bytes = await proc.communicate()
        else:
            stdout_bytes, stderr_bytes = await asyncio.wait_for(proc.communicate(), timeout)
    except asyncio.TimeoutError as exc:
        proc.kill()
        stdout_bytes, stderr_bytes = await proc.communicate()
        raise PortDetectiveExecutionError(
            f"port-detective timed out after {timeout} seconds",
            returncode=-1,
            stderr=_decode_output(stderr_bytes).strip(),
            stdout=_decode_output(stdout_bytes).strip(),
        ) from exc
    except FileNotFoundError as exc:
        raise PortDetectiveNotFoundError(f"Failed to execute {bin_path}: {exc}") from exc
    except OSError as exc:
        raise PortDetectiveExecutionError(
            f"Failed to execute {bin_path}: {exc}",
            returncode=exc.errno or -1,
        ) from exc

    stdout_str = stdout_bytes.decode("utf-8", errors="replace").strip()
    stderr_str = stderr_bytes.decode("utf-8", errors="replace").strip()

    if proc.returncode != 0:
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


# ============================ Synchronous API ============================


def inspect_port(
    port: int,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> PortDiagnostic:
    """
    Inspect a single port and retrieve process health and framework fingerprint.
    """
    _validate_port(port)
    data = _run_cli_sync(["check", str(port)], binary_path=binary_path, timeout=timeout)
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for check, got {type(data)}", returncode=0)
    return PortDiagnostic.from_dict(data)


def scan_ports(
    ports: Optional[List[int]] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> List[PortDiagnostic]:
    """
    Concurrently scan specified development ports (or default dev ports).
    """
    args = ["scan"]
    if ports:
        for port in ports:
            _validate_port(port)
        ports_str = ",".join(str(p) for p in ports)
        args.extend(["--ports", ports_str])

    data = _run_cli_sync(args, binary_path=binary_path, timeout=timeout)
    if not isinstance(data, list):
        if data is None:
            return []
        raise PortDetectiveExecutionError(f"Expected list response for scan, got {type(data)}", returncode=0)
    return [PortDiagnostic.from_dict(item) for item in data]


def free_port(
    port: int,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> KillResult:
    """
    Terminate the process occupying the specified port.
    Set dry_run=False to actually kill the process.
    """
    _validate_port(port)
    args = ["kill", str(port), "-y"]
    if dry_run:
        args.append("--dry-run")

    data = _run_cli_sync(args, binary_path=binary_path, timeout=timeout)
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for kill, got {type(data)}", returncode=0)
    return KillResult.from_dict(data)


def clean_project(
    path: Optional[str] = None,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> List[PortDiagnostic]:
    """
    Concurrently scan dev ports and terminate orphan processes belonging to the project path.
    Set dry_run=False to actually kill the processes.
    """
    args = ["clean", "-y"]
    if dry_run:
        args.append("--dry-run")

    target_cwd = path or os.getcwd()
    data = _run_cli_sync(args, cwd=target_cwd, binary_path=binary_path, timeout=timeout)
    if not isinstance(data, list):
        if data is None:
            return []
        raise PortDetectiveExecutionError(f"Expected list response for clean, got {type(data)}", returncode=0)
    return [PortDiagnostic.from_dict(item) for item in data]


# ============================ Asynchronous API ============================


async def async_inspect_port(
    port: int,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> PortDiagnostic:
    """Asynchronously inspect a single port."""
    _validate_port(port)
    data = await _run_cli_async(["check", str(port)], binary_path=binary_path, timeout=timeout)
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for check, got {type(data)}", returncode=0)
    return PortDiagnostic.from_dict(data)


async def async_scan_ports(
    ports: Optional[List[int]] = None,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> List[PortDiagnostic]:
    """Asynchronously concurrently scan ports."""
    args = ["scan"]
    if ports:
        for port in ports:
            _validate_port(port)
        ports_str = ",".join(str(p) for p in ports)
        args.extend(["--ports", ports_str])

    data = await _run_cli_async(args, binary_path=binary_path, timeout=timeout)
    if not isinstance(data, list):
        if data is None:
            return []
        raise PortDetectiveExecutionError(f"Expected list response for scan, got {type(data)}", returncode=0)
    return [PortDiagnostic.from_dict(item) for item in data]


async def async_free_port(
    port: int,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> KillResult:
    """Asynchronously terminate the process occupying the specified port."""
    _validate_port(port)
    args = ["kill", str(port), "-y"]
    if dry_run:
        args.append("--dry-run")

    data = await _run_cli_async(args, binary_path=binary_path, timeout=timeout)
    if not isinstance(data, dict):
        raise PortDetectiveExecutionError(f"Expected dict response for kill, got {type(data)}", returncode=0)
    return KillResult.from_dict(data)


async def async_clean_project(
    path: Optional[str] = None,
    dry_run: bool = True,
    binary_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_CLI_TIMEOUT,
) -> List[PortDiagnostic]:
    """Asynchronously clean project orphan processes."""
    args = ["clean", "-y"]
    if dry_run:
        args.append("--dry-run")

    target_cwd = path or os.getcwd()
    data = await _run_cli_async(args, cwd=target_cwd, binary_path=binary_path, timeout=timeout)
    if not isinstance(data, list):
        if data is None:
            return []
        raise PortDetectiveExecutionError(f"Expected list response for clean, got {type(data)}", returncode=0)
    return [PortDiagnostic.from_dict(item) for item in data]
