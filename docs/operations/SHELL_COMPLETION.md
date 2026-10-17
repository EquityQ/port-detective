# Shell Completion

Build or install `port-detective`, then generate completion scripts with the `completion`
command.

## Bash

```sh
source <(port-detective completion bash)
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

```powershell
port-detective completion powershell | Out-String | Invoke-Expression
```

Add the same command to `$PROFILE` for future sessions.
