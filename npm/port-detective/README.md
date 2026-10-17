# port-detective Node.js Client

A minimal-dependency Node.js client for `port-detective` with synchronous, Promise, and
TypeScript APIs. Published releases install a matching platform CLI package automatically.

## Install

Install the published package from npm:

```bash
npm install port-detective
```

The published package supports Windows, macOS, and Linux on x64 and arm64. Source
checkouts omit generated platform packages; build the CLI locally or set
`PORT_DETECTIVE_BIN` when developing from this directory. `binaryPath` always overrides
automatic resolution, and unsupported platforms can use a CLI on `PATH`.

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
