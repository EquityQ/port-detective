# Community Submissions

Community evidence helps us improve compatibility notes and release decisions. A
submission records observed behavior; it does not create a support guarantee.

## Compatibility reports

Open the [bug report form](../../.github/ISSUE_TEMPLATE/bug_report.yml) and include:

- OS and version, architecture, and whether the host is WSL, a container, or a sandbox.
- Go, Python, and Node.js versions when relevant.
- CLI, SDK, or commit version; include the install source.
- Permission level and whether IPv4, IPv6, or both were tested.
- The exact sanitized command, expected result, observed result, and relevant output.

Remove credentials, tokens, private paths, and production process command lines before
posting. Report security vulnerabilities through the [security policy](SECURITY.md).

## Benchmark evidence

Submit a pull request with the script or command used, or attach the results to a bug
report when the benchmark exposes a regression. Record the commit or package version,
OS and architecture, number of ports, number of repetitions, total runtime, and observed
CPU and memory. Explain listener setup and cleanup so another contributor can reproduce
the measurement without touching unrelated processes.

## Governance and maintenance proposals

Use the [feature request form](../../.github/ISSUE_TEMPLATE/feature_request.yml) for
changes to scope, defaults, support policy, or project maintenance. Include the problem,
proposed outcome, compatibility impact, and the expected maintenance or review cost.
Use a pull request when a concrete implementation or documentation change is ready.

Do not include exploit details in public issues or pull requests. Evidence and proposals
are reviewed against the current [contribution rules](CONTRIBUTING.md) and [platform
policy](../operations/PLATFORM_SUPPORT.md).
