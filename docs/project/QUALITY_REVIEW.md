# Open Source Quality Review

Review date: 2026-10-17. Scope: Go core, CLI, MCP server, Python and Node.js SDKs,
CI/release automation, documentation, and community files.

## Current assessment

`port-detective` is a feature-complete v1.1 development line with clear layering and
strong safety coverage for a local process tool. It is ready for careful use from source
or a locally built binary. It is not a finished public release because the v1.1.0 tag
and published artifact trail do not exist yet.

**Current score: 91/100**

| Area | Score | Evidence | Remaining gap |
| --- | ---: | --- | --- |
| Product usefulness | 18/20 | CLI, MCP, project cleanup, fingerprinting, Python SDK | More real-world compatibility evidence |
| Architecture | 18/20 | Core/CLI/MCP separation, worker pool, protocol-safe stdout, focused CLI files | More contract-level regression coverage |
| Correctness and safety | 19/20 | Race tests, PID checks, listener revalidation, dry-run and JSON confirmation guards | More OS-specific termination fixtures |
| Developer experience | 15/15 | Concise quick start, issue forms, three-OS CI, typed Node client, wheel checks, API drift gate, shell completion | Published npm package and public release evidence |
| Release and supply chain | 14/15 | Atomic verification, immutable Action SHAs, checksums, signatures, SBOM, provenance, PyPI workflow | Actual tag and artifact trail |
| Community operations | 7/10 | Security, conduct, contribution, status, and PR guidance | Named ownership and support history |

## Required before v1.1.0

1. Create the tag only after the release workflow succeeds.
2. Review checksums, signatures, SBOM, and provenance attestation.
3. Run MCP acceptance scripts against the release binary.
4. Verify the matching Python package installation from the published release.
5. Decide whether to publish the Node.js client to npm.

## Quality gates

Every change should pass:

```text
go build ./...
go test -race ./...
go vet ./...
gofmt -l .
python -m unittest discover -s python/tests -v
npm test --prefix npm/port-detective
python scripts/quality/verify_api_reference.py
```

CI enforces reviewed thresholds: Go core/CLI/MCP/fingerprint 60/45/70/70 percent;
Python core/models/overall 60/85/65 percent; Node.js lines/branches/functions
45/65/55 percent. Go and Python coverage reports remain CI artifacts.
