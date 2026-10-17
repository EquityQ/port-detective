# port-detective Python SDK

A lightweight subprocess client for the `port-detective` CLI. It provides synchronous
and `asyncio` APIs for port diagnostics, project fingerprinting, and safe cleanup on
Windows, macOS, and Linux.

## Install

Install the published SDK:

```bash
python -m pip install port-detective
```

From a checkout:

```bash
python -m pip install ./python
go build -o port-detective .
```

The package does not bundle the Go executable. Put a matching CLI on `PATH`, set
`PORT_DETECTIVE_BIN`, or pass `binary_path` to an API call. Keep SDK and CLI versions
aligned because the SDK consumes the CLI JSON contract.

## Quickstart

```python
from port_detective import inspect_port, scan_ports, free_port, clean_project

diagnostic = inspect_port(8080)
occupied = scan_ports([3000, 5173, 8080])
preview = free_port(3000, dry_run=True)
project_listeners = clean_project(dry_run=True)

print(diagnostic.status, len(occupied), preview.message, len(project_listeners))
```

All calls accept an optional timeout in seconds. Process actions default to dry-run;
set `dry_run=False` only after reviewing the target.

### Async API

```python
import asyncio
from port_detective import async_inspect_port, async_scan_ports

async def main():
    diagnostic, ports = await asyncio.gather(
        async_inspect_port(3000),
        async_scan_ports(),
    )
    print(diagnostic.status, len(ports))

asyncio.run(main())
```

## Models

`PortDiagnostic` includes `port`, `is_open`, `protocol`, `pid`,
`process_name`, `cmdline`, `memory_mb`, `cpu_percent`, `status`, and optional
project metadata. `ProjectInfo` includes `name`, `framework`, `path`, and
`version`.

See the [API reference](../docs/project/API_REFERENCE.md), [platform support policy](../docs/operations/PLATFORM_SUPPORT.md),
and [quality review](../docs/project/QUALITY_REVIEW.md) for release constraints.

## License

MIT.
