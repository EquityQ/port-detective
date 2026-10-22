# Troubleshooting

With a CLI built from this checkout, start with `port-detective doctor --json`.
It reports the executable actually running, version, OS/architecture, working
directory, and process visibility for a disposable local listener. It does not
change another process or automatically elevate privileges. `doctor` and the
new workflow commands are unreleased after v1.1.1.

| Symptom | Check |
| --- | --- |
| A port is shown as free but a service is reachable | Confirm a local TCP listener, IPv4/IPv6 binding, network namespace, and the host running the CLI. |
| `permission_required` or empty process fields | The OS restricts process metadata. Retry with appropriate privileges only when understood. |
| `kill` or `clean` refuses to continue | Start with `--dry-run`; confirm the target and project path. JSON termination also needs `--yes`. |
| `clean` finds no project listeners | Check the effective project directory. Use `clean --project /absolute/project --dry-run`; a missing fingerprint or another network namespace can hide targets. `--ports` restricts the selection. |
| MCP cleans the wrong project scope | With the source-built server, supply `project_path` explicitly and review the returned `cwd` and `scope`. v1.1.1 uses the server process working directory. |
| SDK reports an incompatible CLI | Check `port-detective --version` and the configured binary path; use the same major/minor CLI as the SDK, or select the matching binary with `PORT_DETECTIVE_BIN`. |
| `wait` times out | Check `--state`, the timeout, local listener bindings, and the network namespace. Port occupancy is not an application readiness check. |
| Python cannot find the CLI | Allow the first-use GitHub download, or install the matching CLI, add it to `PATH`, set `PORT_DETECTIVE_BIN`, or disable automatic downloads with `PORT_DETECTIVE_AUTO_DOWNLOAD=0`. |
| MCP output cannot be parsed | Keep stdout connected to the MCP client and send logs only to stderr. |
| The command is not found | Add the executable directory to `PATH` and check `port-detective --version`. |

Useful diagnostics:

```sh
port-detective --version
port-detective doctor --json
port-detective check 8080 --json
port-detective scan --ports 3000,5173,8080 --json
go version
python --version
node --version
```

Remove process command lines and private paths before sharing output. For a platform
issue, include the OS, architecture, runtime versions, CLI version, and permission
context in a GitHub issue.
