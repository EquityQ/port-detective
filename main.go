// Command port-detective 是一个跨平台的本地端口占用侦探与进程体检 CLI，
// 同时原生支持 MCP 协议，可作为 AI Agent 的工具直接调用。
//
// 本文件是唯一入口，只负责把控制权交给 internal/cli。
package main

import (
	"port-detective/internal/cli"
	_ "port-detective/internal/fingerprint"
)

func main() {
	cli.Execute()
}
