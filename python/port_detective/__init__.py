"""
port_detective
~~~~~~~~~~~~~~

Lightweight Python SDK for port-detective: inspect ports, detect framework fingerprints,
and clean project-scoped listeners.
"""

from .models import PortDiagnostic, ProjectInfo, KillResult
from .core import (
    DEFAULT_CLI_TIMEOUT,
    inspect_port,
    scan_ports,
    free_port,
    clean_project,
    async_inspect_port,
    async_scan_ports,
    async_free_port,
    async_clean_project,
    find_binary,
    PortDetectiveError,
    PortDetectiveNotFoundError,
    PortDetectiveExecutionError,
)

__version__ = "1.1.0.dev0"
__all__ = [
    "PortDiagnostic",
    "ProjectInfo",
    "KillResult",
    "inspect_port",
    "scan_ports",
    "free_port",
    "clean_project",
    "async_inspect_port",
    "async_scan_ports",
    "async_free_port",
    "async_clean_project",
    "find_binary",
    "PortDetectiveError",
    "PortDetectiveNotFoundError",
    "PortDetectiveExecutionError",
    "DEFAULT_CLI_TIMEOUT",
]
