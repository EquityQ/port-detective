# Contributing

Read the [Code of Conduct](CODE_OF_CONDUCT.md) and [API reference](docs/API_REFERENCE.md)
before changing the project.

## Architecture

1. `internal/core` must not import Cobra or MCP packages.
2. MCP stdout is reserved for JSON-RPC; diagnostics go to stderr.
3. Cross-platform code must use `gopsutil/v3` and the standard library instead of
   shelling out to `netstat`, `lsof`, or `kill`.
4. Permission-limited process metadata must degrade gracefully.

## Local checks

Requirements: Go 1.25+, Python 3.9+, and Node.js 22+ when changing the SDKs.

Run the unified local verification suite:

```bash
python scripts/quality/run_checks.py
```

Or run individual checks directly:

```bash
go build ./...
go test -race ./...
go vet ./...
gofmt -l .
python -m unittest discover -s python/tests -v
npm test --prefix npm/port-detective
python scripts/quality/verify_api_reference.py
```

For MCP changes, build the CLI and run the scripts in `scripts/mcp/`. Use a temporary
local listener for process-action tests.

## Commits and pull requests

Use [Conventional Commits](https://www.conventionalcommits.org/), for example
`fix(cli): preserve JSON output`. Explain compatibility impact for code, CI, release,
or documentation changes. Do not claim a release is available until its tag and
artifacts have been published and verified.
