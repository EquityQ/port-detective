# Reproducible CLI Demo

Build the CLI from the repository root, then run:

```sh
go build -o port-detective .
python scripts/demo/run_demo.py
```

On Windows, the default binary is `port-detective.exe`. Override discovery with `--binary PATH` or `PORT_DETECTIVE_BIN`.

The script opens an ephemeral loopback listener in its own process, calls `check`, `scan`, and `kill --dry-run`, confirms the listener remains reachable, and closes it. It never terminates a process. The same script is intended for Windows, macOS, and Linux with Python 3.9+.

从仓库根目录构建 CLI 后运行上面的命令。Windows 默认查找 `port-detective.exe`，也可通过 `--binary PATH` 或 `PORT_DETECTIVE_BIN` 指定位置。

脚本在自身进程中打开临时回环监听，依次调用 `check`、`scan` 和 `kill --dry-run`，确认监听仍可连接后关闭它。脚本不会终止任何进程，支持 Python 3.9+ 及 Windows、macOS、Linux。
