# port-detective Node.js Client

A minimal-dependency Node.js client for `port-detective` with synchronous, Promise, and
TypeScript APIs. Published releases install a matching platform CLI package automatically.

This checkout documents unreleased improvements after `v1.1.1`. Version checks,
explicit `projectPath`, and cleanup `ports` require this checkout's SDK; cleanup
selection also requires this checkout's CLI. Install both from source as shown
below until the next release. Major/minor compatibility
protects the shared API contract and does not add new flags to an older CLI.

## Install

Install the published package from npm:

```bash
npm install port-detective
```

Published npm installations include the matching platform CLI binary. Users do not
need Go or a local CLI build on the supported platforms.

For unreleased features, run both commands from the repository root:

```bash
npm install ./npm/port-detective
go build -o port-detective .
```

Set `PORT_DETECTIVE_BIN` to the absolute path of that local executable before
using the source SDK. On Windows, build with `-o port-detective.exe`.

The published package supports Windows, macOS, and Linux on x64 and arm64. Source
checkouts omit generated platform packages; contributors should build the CLI locally
or set `PORT_DETECTIVE_BIN` when developing from this directory. `binaryPath` always
overrides automatic resolution, and unsupported platforms can use a CLI on `PATH`.
Resolution order is `binaryPath`, `PORT_DETECTIVE_BIN`, the bundled platform
binary, then `PATH`. External CLIs must share the SDK's major and minor version:
SDK `1.1.x` accepts CLI `1.1.x`, including `1.1.0-dev`. Incompatible or unrecognized
versions raise `PortDetectiveVersionError` with the resolved executable and repair
instructions. Successful checks are cached until that file's identity changes.

The published package also exposes the CLI command through npm:

```bash
npx --no-install port-detective check 3000
npx --no-install port-detective scan --ports 3000,8080
```

After installation, `port-detective` is available from npm scripts and
`node_modules/.bin`. `PORT_DETECTIVE_BIN` overrides the bundled platform binary;
without that override, the wrapper uses its bundled binary and then falls back to
a CLI on `PATH`.

## Use

```js
const {
  inspectPort,
  scanPorts,
  freePort,
  asyncInspectPort,
  cleanProject,
} = require("port-detective");

const diagnostic = inspectPort(3000);
const matches = scanPorts([3000, 8080]);
const preview = freePort(3000); // dryRun defaults to true
const projectPreview = cleanProject({
  projectPath: "/workspace/my-app",
  ports: [3000, 8080],
});

async function inspectAgain() {
  return asyncInspectPort(3000);
}
```

Async methods accept an `AbortSignal`. Process actions require
`{ dryRun: false }`; the wrapper then passes the CLI's `--yes` flag. Type
declarations are included in `index.d.ts`.
Timeouts and aborts also apply to asynchronous CLI version checks, which do not
block the event loop. Both sync and async methods share their timeout budget between
the version check and command execution.

`scanPorts` returns occupied ports with diagnostics in ascending port order. An
omitted list or `[]` scans default development ports. `cleanProject` discovers the
project's TCP listeners, including ports outside the default list; `ports` limits
that selection when supplied and nonempty. `projectPath` selects the project
explicitly; otherwise cleanup uses `cwd` or the caller's current directory.
The directory resolves to the nearest `package.json`, `go.mod`, `pyproject.toml`,
or `requirements.txt` ancestor, or keeps its own path if no manifest exists. A
subdirectory can therefore select its parent project; unrelated nested projects
are excluded.
