## What changed

<!-- Describe the user-visible behavior and the reason for this change. -->

## Validation

- [ ] `go build ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `gofmt` check
- [ ] Python tests, when `python/` changed

## Review checklist

- [ ] The change follows the architecture boundaries in `docs/community/CONTRIBUTING.md`.
- [ ] MCP mode keeps stdout reserved for JSON-RPC and sends diagnostics to stderr.
- [ ] User-facing documentation and release notes were updated when needed.
- [ ] Tests and logs contain no secrets or private host data.

## Compatibility or risk

<!-- Mention platform-specific behavior, migrations, breaking changes, or residual risk. -->
