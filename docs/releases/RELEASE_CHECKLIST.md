# Release Checklist

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

The tag workflow builds GoReleaser artifacts without publishing them, completes the
verification and signing gates, publishes the Python distributions to PyPI, and
creates the GitHub Release as the final publication step:

1. Build the CLI with the tag version and verify `port-detective --version`.
2. Rewrite the temporary Python build metadata to the same version and verify the wheel and source distribution metadata.
3. Verify every GoReleaser SHA-256 entry and require `README.md`, `README_zh.md`, and `LICENSE` in each archive.
4. Run the MCP handshake, tool, and stdout-purity checks against the release candidate.
5. Generate an SPDX JSON SBOM, sign Go archives, Python distributions, checksums, and the SBOM with keyless Cosign.
6. Verify every Cosign signature and publish GitHub build provenance attestations.
7. Publish the Python wheel and source distribution through the configured PyPI Trusted Publisher.
8. Install that exact version from PyPI and exercise it against the matching CLI binary.
9. Create the GitHub Release and upload the already verified archives, Python distributions, SBOM, signatures, and certificates.

Before the first release, configure a PyPI Trusted Publisher for repository
`EquityQ/port-detective`, workflow `.github/workflows/release.yml`, environment
`pypi`, and the `port-detective` project name. The workflow requests only the
short-lived `id-token: write` permission; no PyPI API token is stored in GitHub.

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

Tag 工作流先生成但不发布 GoReleaser 制品，完成校验和签名门禁后发布 Python 分发包到 PyPI，最后创建 GitHub Release：

1. 使用 Tag 版本构建 CLI，并校验 `port-detective --version`。
2. 临时写入相同版本的 Python 构建元数据，并校验 wheel 和源码分发包版本。
3. 校验每一项 GoReleaser SHA-256，并确认每个归档包含 `README.md`、`README_zh.md` 和 `LICENSE`。
4. 使用发布候选二进制运行 MCP 握手、工具和 stdout 纯净性验收。
5. 生成 SPDX JSON SBOM，使用无密钥 Cosign 签名 Go 归档、Python 分发包、校验和与 SBOM。
6. 验证每个 Cosign 签名，并发布 GitHub 构建来源证明。
7. 通过已配置的 PyPI Trusted Publisher 发布 Python wheel 和源码包。
8. 从 PyPI 安装相同版本，并使用匹配的 CLI 二进制执行真实检查。
9. 创建 GitHub Release，上传已验证的归档、Python 分发包、SBOM、签名和证书。

首次发布前，请在 PyPI 为仓库 `EquityQ/port-detective` 配置 Trusted Publisher：
工作流为 `.github/workflows/release.yml`，GitHub 环境为 `pypi`，项目名为
`port-detective`。工作流只申请短期的 `id-token: write` 权限，不在 GitHub 保存 PyPI API Token。

下载者应在解压前验证 `checksums.txt`，并将匹配的 `.sig`、`.pem` 文件和 GitHub attestation 与发布记录一起保存。首个 `v1.1.0` Tag 仍需维护者在审查工作流后执行。
