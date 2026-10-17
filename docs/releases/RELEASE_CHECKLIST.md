# Release Checklist

The tag workflow builds GoReleaser artifacts without publishing them, completes the
verification and signing gates, publishes the Python distributions to PyPI, and
creates the GitHub Release as the final publication step:

1. Build the CLI with the tag version and verify `port-detective --version`.
2. Rewrite the temporary Python build metadata to the same version and verify the wheel and source distribution metadata.
3. Verify every GoReleaser SHA-256 entry and require `README.md` and `LICENSE` in each archive.
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
