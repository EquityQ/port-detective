# Reproducible CLI Demo

Build the CLI from the repository root, then run:

```sh
go build -o port-detective .
python scripts/demo/run_demo.py
```

On Windows, the default binary is `port-detective.exe`. Override discovery with
`--binary PATH` or `PORT_DETECTIVE_BIN`.

The script opens an ephemeral loopback listener in its own process, calls `check`,
`scan`, and `kill --dry-run`, confirms the listener remains reachable, and closes it.
It never terminates an external process and is intended for Windows, macOS, and Linux.
