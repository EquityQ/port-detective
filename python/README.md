# port-detective Python SDK

Lightweight, high-performance Python client wrapper for [`port-detective`](https://github.com/EquityQ/port-detective) — the local port inspector, framework fingerprint engine, and process diagnostic tool.

Uses a zero-CGo **Subprocess Wrapper** architecture, ensuring 100% cross-platform compatibility across Windows, macOS, and Linux without native binary compilation headaches.

---

## Features

- 🔍 **Single Port Inspection**: Check port occupancy, owning PID, memory RSS, CPU%, and command line.
- 🧠 **Framework Fingerprint Detection**: Automatically identify Next.js, Vite, Nuxt, Express, NestJS, FastAPI, Django, Streamlit, Jupyter, Gin, Fiber, etc.
- ⚡ **Concurrent Dev Port Scanning**: Rapidly scan dev ports (`3000`, `8000`, `8080`, `5173`, etc.) using Go worker pools.
- 🧹 **Project Port Clean**: Safely terminate or preview (`dry_run=True`) listeners belonging to the current project path.
- 🚀 **Full Async/Await Support**: Both synchronous and `asyncio` APIs out of the box.
- ⏱️ **Bounded CLI Calls**: API calls use a 30-second timeout by default; pass `timeout=None` for a deliberate unlimited wait.

---

## Installation

```bash
# After a tagged release is published:
python -m pip install port-detective

# Or install this checkout while developing:
python -m pip install ./python
```

The Python package is a thin client and does not bundle the Go executable. The release
workflow publishes the wheel and source distribution through PyPI Trusted Publishing,
then installs the exact version from PyPI and exercises it against the matching CLI.
The v1.1.0 package is not published yet; install a reviewed CLI source build separately
and make it available on `PATH`:

```bash
# Build the reviewed CLI source revision from the repository root
go build -o port-detective .

# Option B: download the matching OS/architecture archive after the release is published
# https://github.com/EquityQ/port-detective/releases
```

For a custom location, set `PORT_DETECTIVE_BIN` or pass `binary_path` to each API call.
Keep the Python SDK and CLI on the same release version because the SDK consumes the
CLI's JSON output contract. The repository currently contains the v1.1 development
line; pin both packages to a reviewed release or commit for production automation.

All public calls accept an optional `timeout` in seconds. Invalid ports and missing
project working directories fail before a child process is started.

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

# 3. Preview project-scoped listener cleanup
preview = clean_project(dry_run=True)
print(f"Found {len(preview)} project-scoped listeners.")

# To actually terminate the selected listeners:
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

See the repository [quality review](../docs/project/QUALITY_REVIEW.md), [roadmap](../docs/project/ROADMAP.md),
and [platform support policy](../docs/operations/PLATFORM_SUPPORT.md) for release and compatibility
status. 中文说明请查看根目录的 [README_zh.md](../README_zh.md) 与[文档中心](../docs/README.md)。

## 中文说明

这是一个通过子进程调用 `port-detective` CLI 的轻量 Python SDK，提供同步和 `asyncio` 异步接口。Tag 工作流会通过 PyPI Trusted Publishing 发布 wheel 和源码包，并从 PyPI 安装相同版本进行真实 CLI 验证；v1.1.0 目前尚未发布。开发时可运行 `python -m pip install ./python`，再把匹配版本的 CLI 放入 `PATH` 或设置 `PORT_DETECTIVE_BIN`。

主要接口包括 `inspect_port`、`scan_ports`、`free_port` 和 `clean_project`；释放或清理操作默认使用 dry-run 预演。SDK 与 CLI 的版本需要保持一致，完整示例请查看上面的 English API quickstart。
