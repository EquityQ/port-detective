"""
port_detective
~~~~~~~~~~~~~~

Lightweight Python SDK for port-detective: inspect ports, detect framework fingerprints,
and clean project-scoped listeners.
"""

from .models import PortDiagnostic, ProjectInfo, KillResult, CleanResult, FreePortResult, DoctorReport
from .core import (
    DEFAULT_CLI_TIMEOUT,
    inspect_port,
    scan_ports,
    free_port,
    clean_project,
    clean_project_result,
    wait_for_port,
    find_free_port,
    doctor,
    async_inspect_port,
    async_scan_ports,
    async_free_port,
    async_clean_project,
    async_clean_project_result,
    async_wait_for_port,
    async_find_free_port,
    async_doctor,
    find_binary,
    download_cli,
    PortDetectiveError,
    PortDetectiveNotFoundError,
    PortDetectiveVersionError,
    PortDetectiveExecutionError,
)

__version__ = "1.5.0"
__all__ = [
    "PortDiagnostic",
    "ProjectInfo",
    "KillResult",
    "CleanResult",
    "FreePortResult",
    "DoctorReport",
    "inspect_port",
    "scan_ports",
    "free_port",
    "clean_project",
    "clean_project_result",
    "wait_for_port",
    "find_free_port",
    "doctor",
    "async_inspect_port",
    "async_scan_ports",
    "async_free_port",
    "async_clean_project",
    "async_clean_project_result",
    "async_wait_for_port",
    "async_find_free_port",
    "async_doctor",
    "find_binary",
    "download_cli",
    "PortDetectiveError",
    "PortDetectiveNotFoundError",
    "PortDetectiveVersionError",
    "PortDetectiveExecutionError",
    "DEFAULT_CLI_TIMEOUT",
]
