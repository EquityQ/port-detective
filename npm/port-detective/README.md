# port-detective Node.js Client

A minimal-dependency Node.js client for `port-detective` with synchronous, Promise, and
TypeScript APIs. Published releases install a matching platform CLI package automatically.

Workflow helpers (`cleanProjectResult`, `waitForPort`, `findFreePort`, `doctor`,
`protocol`, `preset`, `previewId`, `bind_addresses`), version checks, explicit `projectPath`,
cleanup `ports`, and fail-closed port selection are available in `v1.6.0` with the matching CLI.
Major/minor compatibility protects the shared API contract and does not add new flags to
an older CLI.

An explicitly empty port selection is rejected: `scanPorts([])` and
`cleanProject({ ports: [] })` throw `TypeError` instead of silently scanning the default
ports or every project listener. Omit the argument, or pass `undefined`/`null`, to use the
documented default.

## Install

Install the published package from npm:

```bash
npm install port-detective@1.6.0
```

Published npm installations include the matching platform CLI binary. Users do not
need Go or a local CLI build on the supported platforms.

For source development, install this checkout and build the matching CLI from the
repository root:

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
SDK `1.6.x` accepts CLI `1.6.x`, including `1.6.0-dev`, and rejects CLI `1.5.x`. Incompatible or unrecognized
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
  cleanProject,
  waitForPort,
  findFreePort,
  doctor,
  asyncInspectPort,
} = require("port-detective");

const diagnostic = inspectPort(3000);
const udpDiag = inspectPort(5353, { protocol: "udp" });
const matches = scanPorts([3000, 8080]);
const webMatches = scanPorts(undefined, { preset: "web" });
const preview = freePort(3000); // dryRun defaults to true
const projectPreview = cleanProject({
  projectPath: "/workspace/my-app",
  ports: [3000, 8080],
});
const ready = waitForPort(3000, { state: "open", waitTimeout: 10_000 });
const free = findFreePort({ from: 3000, to: 3100 });
const envReport = doctor();

async function inspectAgain() {
  return asyncInspectPort(3000);
}
```

Async methods (`asyncInspectPort`, `asyncScanPorts`, `asyncFreePort`,
`asyncCleanProject`, `asyncCleanProjectResult`, `asyncWaitForPort`, `asyncFindFreePort`,
`asyncDoctor`) accept an `AbortSignal`. Process actions require `{ dryRun: false }`; the wrapper then passes
the CLI's `--yes` flag and supports `{ previewId }` to bind execution to a prior
dry-run snapshot. Type declarations are included in `index.d.ts`. Workflow helpers
(`cleanProjectResult` returning `CleanResult`, `waitForPort`, `findFreePort`, `doctor`,
`protocol: "udp"` across inspect/scan/free, `preset`, `previewId`, and `bind_addresses`)
are included in `v1.6.0`.
Timeouts and aborts also apply to asynchronous CLI version checks, which do not
block the event loop. Both sync and async methods share their timeout budget between
the version check and command execution.

`scanPorts` returns occupied ports with diagnostics in ascending port order. An
omitted list or `null` scans default development ports; an explicitly empty array
throws a `TypeError` instead, so an empty selection can never widen the scan to the
default ports. `cleanProject` discovers the project's TCP listeners, including ports
outside the default list; `ports` limits that selection when supplied and nonempty,
and `{ ports: [] }` likewise throws a `TypeError` rather than silently discovering
everything. `projectPath` selects the project
explicitly; otherwise cleanup uses `cwd` or the caller's current directory.
The directory resolves to the nearest supported project manifest (`package.json`,
`go.mod`, `pyproject.toml`, `requirements.txt`, `Cargo.toml`, `pom.xml`,
`build.gradle`, `build.gradle.kts`, `Gemfile`, `composer.json`,
`deno.json`, `deno.jsonc`, `bunfig.toml`, or `*.csproj`), or keeps its
own path if no manifest exists. A subdirectory can therefore select its parent
project; unrelated nested projects are excluded.
