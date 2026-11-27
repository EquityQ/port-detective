# Scripts

Repository automation and verification helpers grouped by responsibility:

- `scripts/demo/` — Safe, temporary-listener CLI demonstration (`run_demo.py`).
- `scripts/integration/` — Real-binary integration checks for the Python and Node.js SDKs (`verify_python_cli.py`, `verify_node_cli.js`).
- `scripts/mcp/` — End-to-end Stdio JSON-RPC handshake, tool behavior, and stdout-purity acceptance checks (`verify_handshake.py`, `verify_tools.py`, `verify_stdout_purity.py`).
- `scripts/quality/` — Unified local verification runner (`run_checks.py` with `--skip-go`, `--skip-python`, `--skip-node`, `--skip-mypy`, `--skip-tsc`, `--skip-mcp`, `--strict`, plus built-in MCP acceptance and optional `mypy`/`tsc` checks), Go coverage threshold gate (`check_go_coverage.sh`), and CLI/MCP API inventory & drift verification (`generate_api_reference.py`, `verify_api_reference.py`).
- `scripts/release/` — Release preparation and verification for Python distributions, npm platform packages, archive manifests, SHA-256 checksums, and `.sigstore.json` Sigstore bundle completeness (`release_common.py`, `prepare_python_release.py`, `prepare_npm_packages.py`, `verify_artifacts.py --require-bundles --sbom`, `verify_pypi_install.py`).
