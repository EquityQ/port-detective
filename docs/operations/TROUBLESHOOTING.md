# Troubleshooting

| Symptom | Check |
| --- | --- |
| A port is shown as free but a service is reachable | Confirm a local TCP listener, IPv4/IPv6 binding, network namespace, and the host running the CLI. |
| `permission_required` or empty process fields | The OS restricts process metadata. Retry with appropriate privileges only when understood. |
| `kill` or `clean` refuses to continue | Start with `--dry-run`; confirm the target and project path. JSON termination also needs `--yes`. |
| Python cannot find the CLI | Allow the first-use GitHub download, or install the matching CLI, add it to `PATH`, set `PORT_DETECTIVE_BIN`, or disable automatic downloads with `PORT_DETECTIVE_AUTO_DOWNLOAD=0`. |
| MCP output cannot be parsed | Keep stdout connected to the MCP client and send logs only to stderr. |
| The command is not found | Add the executable directory to `PATH` and check `port-detective --version`. |

Useful diagnostics:

```sh
port-detective --version
port-detective check 8080 --json
port-detective scan --ports 3000,5173,8080 --json
go version
python --version
node --version
```

Remove process command lines and private paths before sharing output. For a platform
issue, include the OS, architecture, runtime versions, CLI version, and permission
context in a GitHub issue.
