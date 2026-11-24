# Open Source Quality Review

Review date: 2026-11-24. Scope: Go core, CLI, MCP server, Python and Node.js SDKs,
CI/release automation, documentation, and community files.

## Current assessment

`port-detective` v1.4.0 passed hosted CI and the release workflow. GitHub assets (30
signed files), PyPI distributions, the npm client, and six platform packages were
published; the PyPI and npm clean-install integrations passed.

**Established public-release baseline: 96/100**. Version 1.4.0 closes the cross-process
CLI preview-token gap, adds `bind_addresses`, optional UDP inspection/scanning,
Rust/Java/Kotlin/Ruby/PHP fingerprinting, `core.Inspector` dependency injection, full
Python/Node SDK workflow parity, modular package splits across Go/Python/Node/release
scripts, cross-language schema parity checks, `mypy` and `tsc --noEmit --strict` CI
gates, and raised coverage thresholds.

| Area | Score | Evidence | Remaining gap |
| --- | ---: | --- | --- |
| Product usefulness | 19/20 | TCP/UDP CLI, `bind_addresses`, MCP, project cleanup, 7-ecosystem fingerprinting, full-parity Python & Node SDKs | More real-world compatibility evidence |
| Architecture | 19/20 | Core/CLI/MCP separation, `core.Inspector` DI, modular Go/Python/Node/release layers, `testdata/sdk-cli` isolation, protocol-safe stdout | More long-lived user feedback on DI |
| Correctness and safety | 19/20 | Race tests, mutex-guarded test hooks, cross-process CLI preview keystore + process-isolated MCP tokens, listener revalidation, degraded `wait`/`doctor` honesty, dry-run and JSON confirmation guards | Hosted restricted-environment evidence |
| Developer experience | 15/15 | Concise quick start, unified `run_checks.py`, issue forms, three-OS CI, PEP 561 `py.typed` + `mypy`, strict `tsc` + typed Node client, published platform packages, schema drift gate, shell completion | More user feedback |
| Release and supply chain | 15/15 | v1.4.0 GitHub, PyPI, and npm artifacts with immutable Action SHAs, checksums, signatures, SBOM, provenance, strict six-platform manifest gating in both release and npm workflows, and clean-install checks | None for this release |
| Community operations | 9/10 | Security, conduct, contribution, submission, status, troubleshooting, and PR guidance | Named ownership and long-term support history |

## Release evidence

The [v1.4.0 release workflow](https://github.com/EquityQ/port-detective/actions/runs/45324780732)
passed Go, Python, MCP, strict six-platform archive manifest, signature, provenance, and
PyPI installation checks.
The [npm publication workflow](https://github.com/EquityQ/port-detective/actions/runs/45325148398)
published the client and all six platform packages and passed clean-install CLI integration.

## Quality gates

Local source validation on 2026-11-24 passed Go build, verbose tests, race tests,
vet, formatting, Linux/macOS/Windows-arm64 cross-builds, Python `mypy` (0 issues),
TypeScript `tsc --noEmit --strict`, Python/Node SDK tests and real CLI integrations,
MCP acceptance, cross-language API/schema parity checks, and package builds. Windows
coverage was Go core 83.5%, CLI 77.6%, MCP 82.7%, and fingerprint 80.1%; Python
core/models/overall was 84/97/82%, and Node lines/branches/functions was
90.96/79.69/96.61%.

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

Or individually:

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
