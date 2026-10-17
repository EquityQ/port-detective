# Shell Completion

Language / 语言: English below · [中文摘要](#中文摘要) · [Documentation index / 文档中心](../README.md)

Build or install `port-detective` first. The `completion` command prints scripts for Bash, Zsh, Fish, and PowerShell.

## Bash

For the current session:

```sh
source <(port-detective completion bash)
```

For new sessions on common Linux distributions:

```sh
mkdir -p ~/.local/share/bash-completion/completions
port-detective completion bash > ~/.local/share/bash-completion/completions/port-detective
```

## Zsh

```sh
mkdir -p "${fpath[1]}"
port-detective completion zsh > "${fpath[1]}/_port-detective"
autoload -Uz compinit && compinit
```

## Fish

```sh
mkdir -p ~/.config/fish/completions
port-detective completion fish > ~/.config/fish/completions/port-detective.fish
```

## PowerShell

For the current session:

```powershell
port-detective completion powershell | Out-String | Invoke-Expression
```

To load it for future sessions, add the same command to `$PROFILE`, then restart PowerShell.

## 中文说明

先构建或安装 `port-detective`，然后使用 `completion` 命令生成 Bash、Zsh、Fish 或 PowerShell 补全脚本。上面的命令分别将脚本安装到常见配置目录；PowerShell 可将 `port-detective completion powershell | Out-String | Invoke-Expression` 加入 `$PROFILE`，重启后生效。
