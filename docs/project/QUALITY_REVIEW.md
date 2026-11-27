# Open Source Quality Review

Review date: 2026-11-27. Scope: Go core, CLI, MCP server, Python and Node.js SDKs,
CI/release automation, documentation, and community files.

## Current assessment

`port-detective` v1.5.0 passed hosted CI and the release workflow. GitHub assets (20
signed release files: 10 artifacts paired with 10 `.sigstore.json` Sigstore bundles),
PyPI distributions, the npm client, and six platform packages were published; the PyPI
and npm clean-install integrations passed.

**Established public-release baseline: 97/100**. Version 1.5.0 closes the structured
`clean --json --envelope` / `CleanResult` contract across CLI and both SDKs, extends
termination and preview tokens to optional UDP, moves shared workflows into `internal/core`
and exposes all seven tools on MCP, expands monorepo traversal (8 levels) and runtime
detectors (Deno, Bun, .NET, Fastify/Hono/Elysia/Astro/SvelteKit), completes
`core.Inspector` dependency injection across CLI and MCP, hardens `run_checks.py` with
built-in MCP acceptance, and migrates release signing to Cosign v3 Sigstore bundles.

| Area | Score | Evidence | Remaining gap |
| --- | ---: | --- | --- |
| Product usefulness | 19/20 | TCP/UDP inspect, scan, and release; `clean --envelope`; 7 MCP tools; 10-ecosystem / 8-level monorepo fingerprinting; full-parity Python & Node SDKs | More real-world compatibility evidence |
| Architecture | 20/20 | Pure `internal/core` workflows, complete `core.Inspector` DI across core/CLI/MCP without blank init imports, modular Go/Python/Node/release layers, protocol-safe stdout | None for this baseline |
| Correctness and safety | 19/20 | Race tests, mutex-guarded test hooks, protocol-bound cross-process CLI / process-isolated MCP preview tokens, listener revalidation, fail-closed selections | Hosted restricted-environment evidence |
| Developer experience | 15/15 | Concise quick start, unified `run_checks.py` with MCP acceptance, issue forms, three-OS CI, PEP 561 `py.typed`, strict `tsc`, published platform packages, schema drift gate | More user feedback |
| Release and supply chain | 15/15 | v1.5.0 GitHub, PyPI, and npm artifacts with immutable Action SHAs, checksums, `.sigstore.json` Sigstore bundles + completeness gate, SBOM, provenance, and clean-install checks | None for this release |
| Community operations | 9/10 | Security, conduct, contribution, submission, status, troubleshooting, and PR guidance | Named ownership and long-term support history |

## Release evidence

The [v1.5.0 release workflow](https://github.com/EquityQ/port-detective/actions/runs/45951344497)
passed Go, Python, MCP, strict six-platform archive manifest, Cosign Sigstore bundle
signing/verification/completeness, provenance, and PyPI installation checks.
The [npm publication workflow](https://github.com/EquityQ/port-detective/actions/runs/45951693084)
published the client and all six platform packages and passed clean-install CLI integration.

## Quality gates

Local validation on `v1.5.0` source passed Go build, race tests, vet, formatting, Python
49 SDK tests, 27 release verifier tests, Node.js 19 client tests (91.69% lines, 81.20%
branches, 98.36% functions), real-CLI Python/Node integration, all three MCP acceptance
checks (7 tools), cross-language API/schema parity checks, and `npm pack --dry-run`.
Local Step 38 verification before the version bump also passed Linux/macOS/Windows-arm64
cross-builds and Windows Go coverage thresholds (`core` 81.1%, `cli` 81.1%, `mcpserver`
82.8%, `fingerprint` 79.9%); hosted three-OS CI verified Python `mypy` (and 70/90/70%
coverage), TypeScript `tsc --noEmit --strict`, Go `govulncheck`, and `pip-audit`.

Cross-compilation passed for Linux amd64, macOS amd64, and Windows arm64; hosted CI and
release workflows supplied three-OS runtime verification for Go, Python, and Node.js.
IPv6 loopback was unavailable on this local host, restricted-environment checks remained
opt-in, and the MCP Python listener's project-match assertion was capability-skipped
locally because working-directory metadata was unavailable. Controlled core/MCP tests
still cover positive project matching and disposable-child termination.

Every change should pass:

```text
python scripts/quality/run_checks.py
```

`run_checks.py` runs Go checks, builds a temporary CLI to run MCP acceptance checks, checks
API/schema parity, invokes `mypy` / `tsc` when installed (`--strict` requires them), and
runs Python, release-verifier, and Node.js test suites. Or run checks individually:

```text
go build ./...
go test -race ./...
go vet ./...
gofmt -l .
python -m mypy python/port_detective
python -m unittest discover -s python/tests -v
npm test --prefix npm/port-detective
python scripts/quality/verify_api_reference.py
```

CI enforces reviewed thresholds: Go core/CLI/MCP/fingerprint 75/65/75/75 percent;
Python core/models/overall 70/90/70 percent plus `mypy`; Node.js lines/branches/functions
75/70/80 percent plus `tsc --noEmit --strict` on `index.d.ts`; and cross-language
`PortDiagnostic` schema parity across Go, Python, TypeScript, and
`docs/project/API_REFERENCE.md`. Go and Python coverage reports remain CI artifacts.
