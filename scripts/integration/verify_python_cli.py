"""Exercise the Python SDK against a real port-detective CLI binary."""

from __future__ import annotations

import asyncio
import os
import socket
from typing import Optional

from port_detective import (
    async_clean_project_result,
    async_inspect_port,
    clean_project_result,
    doctor,
    find_free_port,
    free_port,
    inspect_port,
    scan_ports,
    wait_for_port,
)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


async def verify_async(port: int, binary_path: Optional[str]) -> None:
    diagnostic = await async_inspect_port(port, binary_path=binary_path)
    require(diagnostic.port == port, "async inspect returned the wrong port")
    require(diagnostic.is_open, "async inspect did not detect the listener")

    clean_res = await async_clean_project_result(dry_run=True, binary_path=binary_path)
    require(clean_res.dry_run is True, "async clean_project_result did not preserve dry-run mode")
    require(isinstance(clean_res.cleaned, list), "async clean_project_result did not return a list")
    require(clean_res.count == len(clean_res.cleaned), "async clean_project_result count mismatch")
    require(bool(clean_res.cwd), "async clean_project_result omitted cwd")
    require(bool(clean_res.scope), "async clean_project_result omitted scope")
    if clean_res.count > 0:
        require(bool(clean_res.preview_id), "async clean_project_result omitted preview_id for occupied scope")


def main() -> int:
    binary_path = os.environ.get("PORT_DETECTIVE_BIN")

    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        listener.bind(("127.0.0.1", 0))
        listener.listen(16)
        port = listener.getsockname()[1]

        diagnostic = inspect_port(port, binary_path=binary_path)
        require(diagnostic.port == port, "inspect returned the wrong port")
        require(diagnostic.is_open, "inspect did not detect the listener")
        require(diagnostic.protocol == "tcp", "inspect returned a non-TCP protocol")
        require(diagnostic.pid > 0, "inspect did not resolve the listener PID")

        scanned = scan_ports([port], binary_path=binary_path)
        require(len(scanned) == 1 and scanned[0].port == port, "scan did not return the listener")
        require(scanned[0].pid == diagnostic.pid, "scan and inspect resolved different PIDs")

        preview = free_port(port, dry_run=True, binary_path=binary_path)
        require(preview.dry_run, "free_port did not preserve dry-run mode")
        require(preview.diagnostic is not None, "dry-run response omitted its diagnostic")
        require(preview.diagnostic.pid == diagnostic.pid, "dry-run resolved a different PID")
        require(bool(preview.preview_id), "dry-run response omitted preview_id")
        require(bool(diagnostic.bind_addresses), "inspect did not report bind_addresses")

        waited = wait_for_port(port, state="open", wait_timeout=2.0, interval=0.05, binary_path=binary_path)
        require(waited.is_open and waited.port == port, "wait_for_port did not detect the listener")

        suggestion = find_free_port(port_from=45000, port_to=45100, binary_path=binary_path)
        require(45000 <= suggestion.port <= 45100, "find_free_port returned out-of-range port")

        report = doctor(binary_path=binary_path)
        require(report.status in {"ok", "degraded"}, f"unexpected doctor status: {report.status}")

        clean_res = clean_project_result(dry_run=True, binary_path=binary_path)
        require(clean_res.dry_run is True, "clean_project_result did not preserve dry-run mode")
        require(isinstance(clean_res.cleaned, list), "clean_project_result did not return a cleaned list")
        require(clean_res.count == len(clean_res.cleaned), "clean_project_result count mismatch")
        require(bool(clean_res.cwd), "clean_project_result omitted cwd")
        require(bool(clean_res.scope), "clean_project_result omitted scope")
        if clean_res.count > 0:
            require(bool(clean_res.preview_id), "clean_project_result omitted preview_id for occupied scope")

        asyncio.run(verify_async(port, binary_path))

    print(f"verified Python SDK against CLI for TCP port {port}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
