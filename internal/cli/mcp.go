package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/EquityQ/port-detective/internal/mcpserver"
	"github.com/spf13/cobra"
)

// mcp starts the MCP Stdio server.
func newMCPCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "以 MCP 协议 Stdio 服务端模式启动，供 AI Agent 调用 / Start MCP Stdio server",
		Long: strings.Join([]string{
			"以 MCP（Model Context Protocol）Stdio 服务端模式启动。",
			"",
			"启动后通过标准输入输出与 AI 客户端通信，注册以下工具：",
			"  inspect_port           体检单个端口，返回占用进程全景体检报告与框架指纹",
			"  scan_dev_ports         并发扫描开发端口（默认覆盖 DefaultDevPorts）",
			"  free_port              释放端口占用进程（默认 dry_run=true 安全预演）",
			"  clean_current_project  一键清理当前项目占用的开发端口进程",
			"",
			"注意：Stdio 模式下标准输出专供协议通信，诊断信息一律写入标准错误。",
		}, "\n"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.jsonOutput {
				return writeJSON(stdout(cmd), mcpserver.Describe())
			}

			perr := newPalette(os.Stderr)
			fmt.Fprintf(os.Stderr, "%s port-detective 已启动（MCP Stdio），等待客户端连接…\n",
				perr.paint(perr.dim, "·"))
			fmt.Fprintf(os.Stderr, "  已注册工具：inspect_port / scan_dev_ports / free_port / clean_current_project\n")

			ctx, cancel := mcpserver.WaitForSignal(cmd.Context())
			defer cancel()

			if err := mcpserver.RunStdioServer(ctx); err != nil {
				return fmt.Errorf("MCP 服务端异常退出：%w", err)
			}
			return nil
		},
	}
	return cmd
}
