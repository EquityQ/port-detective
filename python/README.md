# port-detective Python SDK

Lightweight, high-performance Python client wrapper for [`port-detective`](https://github.com/EquityQ/port-detective) — the local port inspector, framework fingerprint engine, and process diagnostic tool.

Uses a zero-CGo **Subprocess Wrapper** architecture, ensuring 100% cross-platform compatibility across Windows, macOS, and Linux without native binary compilation headaches.

---

## Features

- 🔍 **Single Port Inspection**: Check port occupancy, owning PID, memory RSS, CPU%, and command line.
- 🧠 **Framework Fingerprint Detection**: Automatically identify Next.js, Vite, Nuxt, Express, NestJS, FastAPI, Django, Streamlit, Jupyter, Gin, Fiber, etc.
- ⚡ **Concurrent Dev Port Scanning**: Rapidly scan dev ports (`3000`, `8000`, `8080`, `5173`, etc.) using Go worker pools.
- 🧹 **One-Click Project Port Clean**: Safely terminate or preview (`dry_run=True`) orphan processes belonging strictly to the current working directory.
- 🚀 **Full Async/Await Support**: Both synchronous and `asyncio` APIs out of the box.

---

## Installation

```bash
pip install port-detective
```

> **Prerequisite**: Ensure `port-detective` executable is installed and available in your system `PATH`, or specify its path via the `PORT_DETECTIVE_BIN` environment variable or the `binary_path` argument.

---

## Quickstart

### Synchronous API

```python
from port_detective import inspect_port, scan_ports, free_port, clean_project

# 1. Inspect a specific port
diag = inspect_port(8080)
if diag.is_open:
    print(f"Port 8080 is occupied by {diag.process_name} (PID: {diag.pid})")
    print(f"Memory: {diag.memory_mb:.1f} MB, CPU: {diag.cpu_percent:.1f}%")
    if diag.project:
        print(f"Framework: {diag.project.framework}, Project: {diag.project.name}")
else:
    print("Port 8080 is free!")

# 2. Concurrently scan high-frequency dev ports
occupied = scan_ports([3000, 5173, 8000, 8080])
for item in occupied:
    fw = item.project.framework if item.project else "Unknown"
    print(f"Port {item.port} -> {item.process_name} [{fw}] (PID {item.pid})")

# 3. Clean orphan processes belonging to the current project (safe dry-run by default)
preview = clean_project(dry_run=True)
print(f"Found {len(preview)} orphan processes to clean.")

# To actually terminate the orphan processes:
# cleaned = clean_project(dry_run=False)

# 4. Release a specific port
res = free_port(3000, dry_run=True)
print(res.message)
```

### Asynchronous API (`asyncio`)

```python
import asyncio
from port_detective import async_inspect_port, async_scan_ports, async_clean_project

async def main():
    # Inspect multiple ports concurrently
    d1, d2 = await asyncio.gather(
        async_inspect_port(3000),
        async_inspect_port(8080)
    )
    print("3000:", d1.status, "8080:", d2.status)

    # Scan dev ports
    ports = await async_scan_ports()
    print(f"Detected {len(ports)} occupied ports.")

asyncio.run(main())
```

---

## Data Models

### `PortDiagnostic`
- `port: int` - Port number (1-65535)
- `is_open: bool` - Whether the port is currently listening
- `protocol: str` - `"tcp"`
- `pid: int` - Owning process PID
- `process_name: Optional[str]` - Executable or process name
- `cmdline: Optional[str]` - Startup command line arguments
- `memory_mb: float` - Resident memory (RSS) in megabytes
- `cpu_percent: float` - CPU utilization percentage
- `status: str` - `"occupied"`, `"free"`, or `"permission_required"`
- `project: Optional[ProjectInfo]` - Detected framework and project metadata

### `ProjectInfo`
- `name: Optional[str]` - Package / module name
- `framework: Optional[str]` - Recognized framework (`"Next.js"`, `"FastAPI"`, `"Gin"`, etc.)
- `path: Optional[str]` - Project root path
- `version: Optional[str]` - Project version from manifest

---

## License

MIT License.
