# Open Source Quality Review

Review date: 2026-10-22. Scope: Go core, CLI, MCP server, Python and Node.js SDKs,
CI/release automation, documentation, and community files.

## Current assessment

`port-detective` v1.3.0 passed hosted CI and the release workflow. GitHub assets,
PyPI distributions, the npm client, and six platform packages were published; the
PyPI and npm clean-install integrations passed.

**Established public-release baseline: 94/100**. This score remains unchanged;
the v1.3.0 release adds current hosted publication and install evidence, and the
preview-token work closes the previously noted listener revalidation gap for
destructive actions.

| Area | Score | Evidence | Remaining gap |
| --- | ---: | --- | --- |
| Product usefulness | 18/20 | CLI, MCP, project cleanup, fingerprinting, Python SDK | More real-world compatibility evidence |
| Architecture | 18/20 | Core/CLI/MCP separation, worker pool, protocol-safe stdout, focused CLI files | More contract-level regression coverage |
| Correctness and safety | 19/20 | Race tests, PID-reuse and protected-PID checks, signed preview-token binding for destructive actions, listener revalidation, dry-run and JSON confirmation guards | Hosted restricted-environment evidence |
| Developer experience | 15/15 | Concise quick start, issue forms, three-OS CI, typed Node client, published platform packages, wheel checks, API drift gate, shell completion | More user feedback |
| Release and supply chain | 15/15 | v1.3.0 GitHub, PyPI, and npm artifacts with immutable Action SHAs, checksums, signatures, SBOM, provenance, exact-platform-manifest gating, and clean-install checks | None for this release |
| Community operations | 7/10 | Security, conduct, contribution, submission, status, and PR guidance | Named ownership and support history |

## Release evidence

The [v1.3.0 release workflow](https://github.com/EquityQ/port-detective/actions/runs/41576379395)
passed Go, Python, MCP, strict six-platform archive manifest, signature, provenance, and PyPI installation checks.
The [npm publication workflow](https://github.com/EquityQ/port-detective/actions/runs/41576720972)
published the client and platform packages and passed clean-install CLI integration.

## Quality gates

Local source validation on 2026-10-22 passed Go build, verbose tests, race tests,
vet, formatting, Python/Node SDK tests and real CLI integrations, MCP acceptance,
API inventory checks, and package builds. Windows coverage was core 81.9%, CLI
74.0%, MCP 84.2%, and fingerprint 75.2%; Python core/models/overall was
73/88/73%, and Node lines/branches/functions was 84.02/71.89/95%.

Cross-compilation passed for Linux amd64, macOS amd64, and Windows arm64. This
is local cross-compilation evidence; hosted SDK and CLI release workflows supply
the runtime evidence for the published packages.
IPv6 loopback was unavailable on this host, restricted-environment checks remained
opt-in, and the MCP Python listener's project-match assertion was capability-skipped
because its working-directory metadata was unavailable. Controlled core/MCP tests
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
75/70/80 percent plus `tsc --noEmit` on `index.d.ts`; and cross-language `PortDiagnostic`
schema parity across Go, Python, TypeScript, and `docs/project/API_REFERENCE.md`. Go and
Python coverage reports remain CI artifacts.
