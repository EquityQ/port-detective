# port-detective Node.js Client

[中文说明](#中文说明)

Zero-dependency Node.js wrapper for the `port-detective` CLI. It provides sync and
Promise APIs with TypeScript declarations. The package does not bundle the CLI; use
the same reviewed CLI version and set `PORT_DETECTIVE_BIN` when it is not on `PATH`.

## Install

Install from a checkout while the npm registry release is pending:

```bash
npm install ./npm/port-detective
go build -o port-detective .
```

Keep the client and CLI on the same reviewed release. The CLI must be installed
separately on Windows, macOS, or Linux. This package is not published to npm yet.

## Use

```js
const {
  inspectPort,
  scanPorts,
  freePort,
  asyncInspectPort,
} = require("port-detective");

const diagnostic = inspectPort(3000);
const matches = scanPorts([3000, 8080]);
const preview = freePort(3000); // dryRun defaults to true

async function inspectAgain() {
  return asyncInspectPort(3000);
}
```

The async APIs accept an `AbortSignal`. Process actions require an explicit
`{ dryRun: false }` to proceed; the wrapper then passes the CLI's `--yes` flag.
API types are included in `index.d.ts`.

## 中文说明

这是一个零运行时依赖的 Node.js CLI 包装器，提供同步 API、Promise API 和
TypeScript 类型声明。npm 包不包含 CLI 二进制；请单独安装同一审核版本的 CLI，
并在 CLI 不在 `PATH` 时设置 `PORT_DETECTIVE_BIN`。

正式 npm registry 版本发布前，可从仓库安装：

```bash
npm install ./npm/port-detective
go build -o port-detective .
```

`inspectPort`、`scanPorts`、`freePort`、`cleanProject` 均有对应的 `async*` API。
释放或清理操作默认使用 dry-run；只有显式设置 `{ dryRun: false }` 才会请求 CLI
执行操作。支持 Windows、macOS 和 Linux，CLI 需要另外安装且应与客户端版本匹配。
