# Release Checklist

The tag workflow builds GoReleaser artifacts without publishing them, completes the
verification and signing gates, publishes the Python distributions to PyPI, and
creates the GitHub Release. It then explicitly dispatches the npm workflow with the
same release tag to publish the Node.js client and six platform packages:

1. Build the CLI with the tag version and verify `port-detective --version`.
2. Rewrite the temporary Python build metadata to the same version and verify the wheel and source distribution metadata.
3. Verify every GoReleaser SHA-256 entry and require `README.md` and `LICENSE` in each archive.
4. Run the MCP handshake, tool, and stdout-purity checks against the release candidate.
5. Generate an SPDX JSON SBOM, sign Go archives, Python distributions, checksums, and the SBOM with keyless Cosign.
6. Verify every Cosign signature and publish GitHub build provenance attestations.
7. Publish the Python wheel and source distribution through the configured PyPI Trusted Publisher.
8. Install that exact version from PyPI and exercise it against the matching CLI binary.
9. Create the GitHub Release and upload the already verified archives, Python distributions, SBOM, signatures, and certificates.
10. Dispatch the npm workflow with the release tag; it stages the six platform packages from verified Go archives, publishes them with provenance, and publishes the client with exact optional dependency versions.
11. Install the published npm client without `PORT_DETECTIVE_BIN` and run the Node.js CLI integration check.

The v1.1.1 release configured a PyPI Trusted Publisher for repository
`EquityQ/port-detective`, workflow `.github/workflows/release.yml`, environment
`pypi`, and the `port-detective` project name. The workflow requests only the
short-lived `id-token: write` permission; no PyPI API token is stored in GitHub.

The v1.1.1 release configured Trusted Publishers for the main `port-detective`
package and each platform package (`port-detective-cli-win32-x64`,
`port-detective-cli-win32-arm64`, `port-detective-cli-darwin-x64`,
`port-detective-cli-darwin-arm64`, `port-detective-cli-linux-x64`, and
`port-detective-cli-linux-arm64`). Use workflow `.github/workflows/npm-publish.yml`
and the `npm` environment. No npm token is stored in GitHub.

Run the local version and package check with:

```bash
python scripts/release/verify_artifacts.py \
  --expected-version 1.2.0 \
  --cli ./port-detective \
  --python-root python \
  --wheel-dir python/dist
```

Downloaders should verify `checksums.txt` before extraction and keep the matching
`.sig`/`.pem` files and GitHub attestation with the release record.

The `v1.1.1` tag and release are complete. The current target is `v1.2.0`; follow
the release and npm runs through completion, then verify both registry versions
and clean installs before recording publication as complete.
