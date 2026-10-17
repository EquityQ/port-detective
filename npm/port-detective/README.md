# port-detective Node.js Client

A zero-dependency Node.js wrapper for the `port-detective` CLI with synchronous,
Promise, and TypeScript APIs. The package does not bundle the CLI.

## Install

Install from a checkout while the npm release is pending:

```bash
npm install ./npm/port-detective
go build -o port-detective .
```

Install the matching CLI separately on Windows, macOS, or Linux. Set
`PORT_DETECTIVE_BIN` when it is not on `PATH`. Keep client and CLI versions aligned.

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

Async methods accept an `AbortSignal`. Process actions require
`{ dryRun: false }`; the wrapper then passes the CLI's `--yes` flag. Type
declarations are included in `index.d.ts`.

The package is available from this checkout and is not published to npm yet.
