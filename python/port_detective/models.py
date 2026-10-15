"""
port_detective.models
~~~~~~~~~~~~~~~~~~~~~

Data models representing port diagnostics, project fingerprints, and termination results.
"""

from dataclasses import dataclass, field, asdict
from typing import Optional, Dict, Any, List


@dataclass
class ProjectInfo:
    """Metadata regarding project identity and framework fingerprint."""
    name: Optional[str] = None
    framework: Optional[str] = None
    path: Optional[str] = None
    version: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Optional[Dict[str, Any]]) -> Optional["ProjectInfo"]:
        if not data or not isinstance(data, dict):
            return None
        return cls(
            name=data.get("name"),
            framework=data.get("framework"),
            path=data.get("path"),
            version=data.get("version"),
        )

    def to_dict(self) -> Dict[str, Any]:
        return {k: v for k, v in asdict(self).items() if v is not None}


@dataclass
class PortDiagnostic:
    """Full diagnostic report for a single local port and owning process."""
    port: int
    is_open: bool
    protocol: str = "tcp"
    pid: int = 0
    process_name: Optional[str] = None
    cmdline: Optional[str] = None
    memory_mb: float = 0.0
    cpu_percent: float = 0.0
    status: str = "free"
    project: Optional[ProjectInfo] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "PortDiagnostic":
        return cls(
            port=int(data.get("port", 0)),
            is_open=bool(data.get("is_open", False)),
            protocol=data.get("protocol", "tcp"),
            pid=int(data.get("pid", 0)),
            process_name=data.get("process_name"),
            cmdline=data.get("cmdline"),
            memory_mb=float(data.get("memory_mb", 0.0)),
            cpu_percent=float(data.get("cpu_percent", 0.0)),
            status=data.get("status", "free"),
            project=ProjectInfo.from_dict(data.get("project")),
        )

    def to_dict(self) -> Dict[str, Any]:
        d = asdict(self)
        if self.project:
            d["project"] = self.project.to_dict()
        return d


@dataclass
class KillResult:
    """Outcome of terminating or dry-running a port process."""
    port: int
    dry_run: bool
    message: str
    diagnostic: Optional[PortDiagnostic] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "KillResult":
        diag = None
        if "diagnostic" in data and isinstance(data["diagnostic"], dict):
            diag = PortDiagnostic.from_dict(data["diagnostic"])
        return cls(
            port=int(data.get("port", 0)),
            dry_run=bool(data.get("dry_run", True)),
            message=data.get("message", ""),
            diagnostic=diag,
        )

    def to_dict(self) -> Dict[str, Any]:
        d = asdict(self)
        if self.diagnostic:
            d["diagnostic"] = self.diagnostic.to_dict()
        return d
