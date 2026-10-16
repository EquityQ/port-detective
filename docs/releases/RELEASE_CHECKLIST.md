# Release Checklist

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

The tag workflow publishes GoReleaser archives and a matching Python wheel after
these checks pass:

1. Build the CLI with the tag version and verify `port-detective --version`.
2. Rewrite the temporary Python build metadata to the same version and verify the wheel metadata.
3. Verify every GoReleaser SHA-256 entry and require `README.md`, `README_zh.md`, and `LICENSE` in each archive.
4. Run the MCP handshake, tool, and stdout-purity checks against the release candidate.
5. Generate an SPDX JSON SBOM, sign Go archives, Python distributions, checksums, and the SBOM with keyless Cosign, and publish GitHub build provenance attestations.

Run the local version and package check with:

```bash
python scripts/release/verify_artifacts.py \
  --expected-version 1.1.0 \
  --cli ./port-detective \
  --python-root python \
  --wheel-dir python/dist
```

Downloaders should verify `checksums.txt` before extraction and keep the matching
`.sig`/`.pem` files and GitHub attestation with the release record. The first
`v1.1.0` tag remains a maintainer action after the workflow is reviewed.

## 中文摘要

Tag 工作流在以下门禁通过后发布 GoReleaser 压缩包和匹配版本的 Python wheel：

1. 使用 Tag 版本构建 CLI，并校验 `port-detective --version`。
2. 临时写入相同版本的 Python 构建元数据，并校验 wheel 内版本。
3. 校验每一项 GoReleaser SHA-256，并确认每个归档包含 `README.md`、`README_zh.md` 和 `LICENSE`。
4. 使用发布候选二进制运行 MCP 握手、工具和 stdout 纯净性验收。
5. 生成 SPDX JSON SBOM，使用无密钥 Cosign 签名 Go 归档、Python 分发包、校验和与 SBOM，并发布 GitHub 构建来源证明。

下载者应在解压前验证 `checksums.txt`，并将匹配的 `.sig`、`.pem` 文件和 GitHub attestation 与发布记录一起保存。首个 `v1.1.0` Tag 仍需维护者在审查工作流后执行。
