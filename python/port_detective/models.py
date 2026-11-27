"""
port_detective.models
~~~~~~~~~~~~~~~~~~~~~

Data models representing port diagnostics, project fingerprints, termination
results, free-port suggestions, and environment doctor reports.
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
    bind_addresses: List[str] = field(default_factory=list)
    listener_table_missing: bool = False
    post_action_status: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "PortDiagnostic":
        raw_addrs = data.get("bind_addresses")
        addrs = [str(a) for a in raw_addrs] if isinstance(raw_addrs, list) else []
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
            bind_addresses=addrs,
            listener_table_missing=bool(data.get("listener_table_missing", False)),
            post_action_status=data.get("post_action_status"),
        )

    def to_dict(self) -> Dict[str, Any]:
        d = asdict(self)
        if self.project:
            d["project"] = self.project.to_dict()
        if not self.bind_addresses:
            d.pop("bind_addresses", None)
        if not self.listener_table_missing:
            d.pop("listener_table_missing", None)
        if self.post_action_status is None:
            d.pop("post_action_status", None)
        return d


@dataclass
class KillResult:
    """Outcome of terminating or dry-running a port process."""
    port: int
    dry_run: bool
    message: str
    diagnostic: Optional[PortDiagnostic] = None
    pre_action_diagnostic: Optional[PortDiagnostic] = None
    post_action_status: Optional[str] = None
    preview_id: Optional[str] = None
    preview_expires_at_unix: Optional[int] = None
    protocol: str = "tcp"

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "KillResult":
        pre_raw = data.get("pre_action_diagnostic")
        diag_raw = data.get("diagnostic")
        snapshot_raw = pre_raw if isinstance(pre_raw, dict) else (diag_raw if isinstance(diag_raw, dict) else None)
        diag = PortDiagnostic.from_dict(snapshot_raw) if snapshot_raw is not None else None
        expires_raw = data.get("preview_expires_at_unix")
        expires = int(expires_raw) if isinstance(expires_raw, int) and not isinstance(expires_raw, bool) else None
        protocol = str(data.get("protocol") or (diag.protocol if diag is not None else "tcp") or "tcp")
        return cls(
            port=int(data.get("port", 0)),
            dry_run=bool(data.get("dry_run", True)),
            message=data.get("message", ""),
            diagnostic=diag,
            pre_action_diagnostic=diag,
            post_action_status=data.get("post_action_status"),
            preview_id=data.get("preview_id"),
            preview_expires_at_unix=expires,
            protocol=protocol,
        )

    def to_dict(self) -> Dict[str, Any]:
        d = asdict(self)
        if self.diagnostic:
            d["diagnostic"] = self.diagnostic.to_dict()
        if self.pre_action_diagnostic:
            d["pre_action_diagnostic"] = self.pre_action_diagnostic.to_dict()
        for key in ("pre_action_diagnostic", "post_action_status", "preview_id", "preview_expires_at_unix"):
            if d.get(key) is None:
                d.pop(key, None)
        return d


@dataclass
class CleanResult:
    """Structured outcome returned by clean_project_result."""
    cwd: str
    scope: str
    dry_run: bool
    count: int
    cleaned: List[PortDiagnostic] = field(default_factory=list)
    preview_id: Optional[str] = None
    preview_expires_at_unix: Optional[int] = None
    message: str = ""
    error: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "CleanResult":
        raw_cleaned = data.get("cleaned")
        cleaned = [
            PortDiagnostic.from_dict(item)
            for item in raw_cleaned
            if isinstance(item, dict)
        ] if isinstance(raw_cleaned, list) else []
        expires_raw = data.get("preview_expires_at_unix")
        expires = int(expires_raw) if isinstance(expires_raw, int) and not isinstance(expires_raw, bool) else None
        return cls(
            cwd=str(data.get("cwd", "")),
            scope=str(data.get("scope", "")),
            dry_run=bool(data.get("dry_run", True)),
            count=int(data.get("count", len(cleaned))),
            cleaned=cleaned,
            preview_id=data.get("preview_id"),
            preview_expires_at_unix=expires,
            message=str(data.get("message", "")),
            error=data.get("error"),
        )

    def to_dict(self) -> Dict[str, Any]:
        d = asdict(self)
        d["cleaned"] = [item.to_dict() for item in self.cleaned]
        for key in ("preview_id", "preview_expires_at_unix", "error"):
            if d.get(key) is None:
                d.pop(key, None)
        return d


@dataclass
class FreePortResult:
    """Loopback bindability snapshot returned by find_free_port."""
    port: int
    protocol: str = "tcp"
    ipv4: bool = True
    ipv6: bool = False
    reserved: bool = False
    message: str = ""

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "FreePortResult":
        return cls(
            port=int(data.get("port", 0)),
            protocol=str(data.get("protocol", "tcp")),
            ipv4=bool(data.get("ipv4", False)),
            ipv6=bool(data.get("ipv6", False)),
            reserved=bool(data.get("reserved", False)),
            message=str(data.get("message", "")),
        )

    def to_dict(self) -> Dict[str, Any]:
        return asdict(self)


@dataclass
class DoctorReport:
    """Environment and listener-visibility diagnostics returned by doctor."""
    version: str
    os: str
    arch: str
    binary_path: str
    cwd: str
    status: str
    metadata_accessible: bool = False
    project_path: Optional[str] = None
    self_check: Optional[PortDiagnostic] = None
    hints: List[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "DoctorReport":
        raw_check = data.get("self_check")
        self_check = PortDiagnostic.from_dict(raw_check) if isinstance(raw_check, dict) else None
        raw_hints = data.get("hints")
        hints = [str(h) for h in raw_hints] if isinstance(raw_hints, list) else []
        return cls(
            version=str(data.get("version", "")),
            os=str(data.get("os", "")),
            arch=str(data.get("arch", "")),
            binary_path=str(data.get("binary_path", "")),
            cwd=str(data.get("cwd", "")),
            status=str(data.get("status", "")),
            metadata_accessible=bool(data.get("metadata_accessible", False)),
            project_path=data.get("project_path"),
            self_check=self_check,
            hints=hints,
        )

    def to_dict(self) -> Dict[str, Any]:
        d = asdict(self)
        if self.self_check:
            d["self_check"] = self.self_check.to_dict()
        if self.project_path is None:
            d.pop("project_path", None)
        if self.self_check is None:
            d.pop("self_check", None)
        return d
