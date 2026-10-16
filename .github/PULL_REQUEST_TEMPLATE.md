Language / 语言: English checklist below · 中文说明见各项标题。

## What changed

## 变更内容

<!-- Describe the user-visible behavior and the reason for this change. -->

## Validation

## 验证结果

- [ ] `go build ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] `gofmt` check
- [ ] Python tests, when `python/` changed

## Review checklist

## 审查清单

- [ ] The change follows the architecture boundaries in `docs/community/CONTRIBUTING.md`.
- [ ] 变更遵守 `docs/community/CONTRIBUTING.md` 中的架构边界。
- [ ] MCP mode keeps stdout reserved for JSON-RPC and sends diagnostics to stderr.
- [ ] User-facing documentation and release notes were updated when needed.
- [ ] Tests and logs contain no secrets or private host data.
- [ ] 测试和日志不包含密钥或私有主机数据。

## Compatibility or risk

## 兼容性或风险

<!-- Mention platform-specific behavior, migrations, breaking changes, or residual risk. -->
