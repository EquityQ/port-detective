# Release Checklist

The tag workflow builds GoReleaser artifacts without publishing them, completes the
verification and signing gates, publishes the Python distributions to PyPI, and
creates the GitHub Release. It then explicitly dispatches the npm workflow with the
same release tag to publish the Node.js client and six platform packages:

1. Build the CLI with the tag version and verify `port-detective --version`.
2. Rewrite the temporary Python build metadata to the same version and verify the wheel and source distribution metadata.
3. Require exactly the six platform archives declared by `.goreleaser.yaml` (linux/darwin/windows x amd64/arm64, tar.gz except windows zip), a SHA-256 entry for each and no entry for a missing archive, and inside every archive the platform executable (`port-detective`, or `port-detective.exe` on windows) alongside `README.md` and `LICENSE`.
4. Run the MCP handshake, tool, and stdout-purity checks against the release build.
5. Generate an SPDX JSON SBOM, sign Go archives, Python distributions, checksums, and the SBOM with keyless Cosign (`--bundle` producing `.sigstore.json` bundles).
6. Verify every Cosign Sigstore bundle, validate bundle completeness (`--require-bundles --sbom`), and publish GitHub build provenance attestations.
7. Publish the Python wheel and source distribution through the configured PyPI Trusted Publisher.
8. Install that exact version from PyPI and exercise it against the matching CLI binary.
9. Create the GitHub Release and upload the already verified archives, Python distributions, SBOM, and `.sigstore.json` Sigstore bundles.
10. Dispatch the npm workflow with the release tag; it stages the six platform packages from verified Go archives, publishes them with provenance, and publishes the client with exact optional dependency versions.
11. Install the published npm client without `PORT_DETECTIVE_BIN` and run the Node.js CLI integration check.

The v1.2.0 release used the configured PyPI Trusted Publisher for repository
`EquityQ/port-detective`, workflow `.github/workflows/release.yml`, environment
`pypi`, and the `port-detective` project name. The workflow requests only the
short-lived `id-token: write` permission; no PyPI API token is stored in GitHub.

The v1.2.0 release used Trusted Publishers for the main `port-detective`
package and each platform package (`port-detective-cli-win32-x64`,
`port-detective-cli-win32-arm64`, `port-detective-cli-darwin-x64`,
`port-detective-cli-darwin-arm64`, `port-detective-cli-linux-x64`, and
`port-detective-cli-linux-arm64`). Use workflow `.github/workflows/npm-publish.yml`
and the `npm` environment. No npm token is stored in GitHub.

Run the local version and package check with:

```bash
python scripts/release/verify_artifacts.py \
  --expected-version 1.5.0 \
  --cli ./port-detective \
  --python-root python \
  --wheel-dir python/dist
```

Downloaders should verify `checksums.txt` before extraction and keep the matching
`.sigstore.json` Sigstore bundle (or `.sig`/`.pem` files for `v1.4.0` and earlier)
and GitHub attestation with the release record.

Run the strict platform-manifest and Sigstore bundle gate against local release artifacts with:

```bash
python scripts/release/verify_artifacts.py \
  --expected-version 1.5.0 \
  --dist dist \
  --checksums dist/checksums.txt \
  --wheel-dir python/dist \
  --sbom port-detective_v1.5.0.spdx.json \
  --require-platform-manifest \
  --require-bundles
```

Both `--require-platform-manifest` and `--require-bundles` are opt-in gates; without
them the earlier checksum and archive checks still run, and historical releases before the
bundle migration retain their original `.sig`/`.pem` verification assets.

The [`v1.5.0` release](https://github.com/EquityQ/port-detective/releases/tag/v1.5.0)
and [release workflow](https://github.com/EquityQ/port-detective/actions/runs/45951344497)
completed successfully. The [npm publication workflow](https://github.com/EquityQ/port-detective/actions/runs/45951693084)
passed its strict six-platform manifest gate and clean-install integration. PyPI 1.5.0
and npm 1.5.0, including all six platform packages, were independently confirmed available.
