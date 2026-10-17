// Package cli implements the terminal interface for port-detective with Cobra.
//
// Design principles:
//   - Keep output restrained with section titles, aligned tables, and simple separators.
//   - Honor NO_COLOR and disable colors when output is redirected or not a TTY.
//   - Support clean, undecorated JSON output for every subcommand.
//
// This package handles data retrieval and presentation; probes live in internal/core.
package cli

import (
	"fmt"
	"os"
	"strings"

	_ "github.com/EquityQ/port-detective/internal/fingerprint"
	"github.com/EquityQ/port-detective/internal/version"
	"github.com/spf13/cobra"
)

type options struct {
	jsonOutput bool
	lang       string
}

func defaultLanguage() string {
	env := strings.ToLower(os.Getenv("LANG") + " " + os.Getenv("LC_ALL") + " " + os.Getenv("LANGUAGE"))
	if strings.Contains(env, "zh") {
		return "zh"
	}
	return "en"
}

func isZh(lang string) bool {
	l := strings.ToLower(strings.TrimSpace(lang))
	return strings.HasPrefix(l, "zh")
}

func NewRootCommand() *cobra.Command {
	opts := &options{}

	root := &cobra.Command{
		Use:     "port-detective",
		Version: version.Value,
		Short:   "本地端口占用侦探与进程体检工具 / Local Port Detective & Process Diagnostic Tool",
		Long: strings.Join([]string{
			"port-detective —— Local Port Detective & Process Diagnostic CLI",
			"",
			"Instantly inspect ports, detect owning processes, identify project frameworks,",
			"and safely clean orphan dev processes.",
			"Native MCP server support for AI Agents (Claude Code / Cursor / Windsurf).",
		}, "\n"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&opts.lang, "lang", defaultLanguage(), "interface language / 界面语言 (en/zh)")
	root.PersistentFlags().BoolVar(&opts.jsonOutput, "json", false, "output in pure JSON format / 以 JSON 格式输出")

	root.AddCommand(
		newCheckCommand(opts),
		newScanCommand(opts),
		newKillCommand(opts),
		newCleanCommand(opts),
		newMCPCommand(opts),
	)
	root.AddCommand(newCompletionCommand(root))
	return root
}

func Execute() {
	root := NewRootCommand()
	if err := root.Execute(); err != nil {
		p := newPalette(os.Stderr)
		fmt.Fprintf(os.Stderr, "%s %v\n", p.paint(p.warn, "错误:"), err)
		os.Exit(1)
	}
}

// newCompletionCommand generates shell completion scripts without requiring a
// separate installation-time tool.
func newCompletionCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion <bash|zsh|fish|powershell>",
		Short: "生成 Shell 补全脚本 / Generate shell completion script",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("需要指定 shell：bash、zsh、fish 或 powershell")
			}
			switch strings.ToLower(args[0]) {
			case "bash", "zsh", "fish", "powershell":
				return nil
			default:
				return fmt.Errorf("不支持的 shell %q：可选 bash、zsh、fish 或 powershell", args[0])
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(args[0]) {
			case "bash":
				return root.GenBashCompletionV2(stdout(cmd), true)
			case "zsh":
				return root.GenZshCompletion(stdout(cmd))
			case "fish":
				return root.GenFishCompletion(stdout(cmd), true)
			case "powershell":
				return root.GenPowerShellCompletion(stdout(cmd))
			default:
				return fmt.Errorf("unreachable shell %q", args[0])
			}
		},
	}
	return cmd
}
