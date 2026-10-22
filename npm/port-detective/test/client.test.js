"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");

const {
  DEFAULT_TIMEOUT,
  PortDetectiveExecutionError,
  PortDetectiveNotFoundError,
  PortDetectiveVersionError,
  inspectPort,
  scanPorts,
  freePort,
  asyncInspectPort,
  cleanProject,
  asyncCleanProject,
  asyncScanPorts,
  asyncFreePort,
} = require("..");
const { binaryRelativePath, platformPackageName } = require("../platform");

test("validates ports before starting the CLI", () => {
  assert.throws(() => inspectPort(0), RangeError);
  assert.throws(() => scanPorts([3000, 65_536]), RangeError);
  assert.throws(() => freePort(true), RangeError);
});

let fixtureRoot;
let fixtureNumber = 0;
test.before(() => {
  fixtureRoot = fs.mkdtempSync(path.join(os.tmpdir(), "port-detective-sdk-"));
  execFileSync(process.env.GO_BINARY || "go", ["build", "-o", path.join(fixtureRoot, "fixture.exe"),
    path.join(__dirname, "fixtures", "cli.go")]);
});
test.after(() => fs.rmSync(fixtureRoot, { recursive: true, force: true }));

function fixture(t, environment = {}) {
  const binary = path.join(fixtureRoot, `fixture-${fixtureNumber++}.exe`);
  fs.copyFileSync(path.join(fixtureRoot, "fixture.exe"), binary);
  if (process.platform !== "win32") fs.chmodSync(binary, 0o700);
  const values = { SDK_TEST_VERSION: "1.2.4", SDK_TEST_VERSION_FAIL: "",
    SDK_TEST_VERSION_DELAY_MS: "0", SDK_TEST_VERSION_LOG: "", SDK_TEST_ARGUMENTS: "", ...environment };
  if (!("SDK_TEST_COMMAND_DELAY_MS" in values)) values.SDK_TEST_COMMAND_DELAY_MS = "0";
  const previous = Object.fromEntries(Object.keys(values).map((key) => [key, process.env[key]]));
  Object.assign(process.env, values);
  t.after(() => {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  });
  return binary;
}

test("accepts compatible patch and versioned development builds", (t) => {
  const binaryPath = fixture(t);
  assert.equal(inspectPort(3000, { binaryPath }).is_open, true);
  const other = fixture(t, { SDK_TEST_VERSION: "1.2.0-dev" });
  assert.equal(inspectPort(3000, { binaryPath: other }).port, 3000);
});

test("rejects incompatible and unrecognizable versions before operations", (t) => {
  const binaryPath = fixture(t);
  for (const version of ["1.1.4", "1.3.0", "2.2.0", "dev", "invalid"]) {
    process.env.SDK_TEST_VERSION = version;
    assert.throws(() => inspectPort(3000, { binaryPath }), (error) =>
      error instanceof PortDetectiveVersionError && /requires CLI 1\.2\.x/.test(error.message));
  }
});

test("caches version by file identity and rechecks a replaced executable", (t) => {
  const log = path.join(fixtureRoot, "version.log");
  const binaryPath = fixture(t, { SDK_TEST_VERSION_LOG: log });
  inspectPort(3000, { binaryPath });
  inspectPort(3000, { binaryPath });
  assert.equal(fs.readFileSync(log, "utf8").trim().split("\n").length, 1);
  const info = fs.statSync(binaryPath);
  fs.utimesSync(binaryPath, info.atime, new Date(info.mtimeMs + 1000));
  process.env.SDK_TEST_VERSION = "2.0.0";
  assert.throws(() => inspectPort(3000, { binaryPath }), PortDetectiveVersionError);
  assert.equal(fs.readFileSync(log, "utf8").trim().split("\n").length, 2);
});

test("preserves version subprocess failures and timeouts", (t) => {
  const binaryPath = fixture(t, { SDK_TEST_VERSION_FAIL: "1" });
  assert.throws(() => inspectPort(3000, { binaryPath }), (error) =>
    error.returncode === 7 && /version unavailable/.test(error.stderr));
  process.env.SDK_TEST_VERSION_FAIL = "";
  process.env.SDK_TEST_VERSION_DELAY_MS = "1000";
  assert.throws(() => inspectPort(3000, { binaryPath, timeout: 30 }), /timed out/);
});

test("async version check is nonblocking, cancellable and bounded by timeout", async (t) => {
  const binaryPath = fixture(t, { SDK_TEST_VERSION_DELAY_MS: "200" });
  let finished = false;
  const pending = asyncInspectPort(3000, { binaryPath }).then((result) => { finished = true; return result; });
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(finished, false);
  assert.equal((await pending).is_open, true);
  const log = path.join(fixtureRoot, "async-versions.log");
  const delayed = fixture(t, { SDK_TEST_VERSION_DELAY_MS: "5000", SDK_TEST_VERSION_LOG: log });
  await assert.rejects(asyncInspectPort(3000, { binaryPath: delayed, timeout: 30 }), /timed out/);
  const controller = new AbortController();
  const aborted = asyncInspectPort(3000, { binaryPath: delayed, signal: controller.signal });
  setTimeout(() => controller.abort(), 30);
  await assert.rejects(aborted, { name: "AbortError" });
  if (fs.existsSync(log)) {
    for (const pid of fs.readFileSync(log, "utf8").trim().split("\n")) {
      assert.throws(() => process.kill(Number(pid), 0), { code: "ESRCH" });
    }
  }
});

test("sync command shares its timeout budget with version discovery", (t) => {
  const binaryPath = fixture(t, { SDK_TEST_VERSION_DELAY_MS: "300", SDK_TEST_COMMAND_DELAY_MS: "600" });
  assert.throws(() => inspectPort(3000, { binaryPath, timeout: 800 }), /timed out/);
});

test("async API rejects incompatible versions and supports project selection", async (t) => {
  const binaryPath = fixture(t, { SDK_TEST_VERSION: "2.0.0" });
  await assert.rejects(asyncInspectPort(3000, { binaryPath }), PortDetectiveVersionError);
  process.env.SDK_TEST_VERSION = "1.2.0";
  const record = path.join(fixtureRoot, "arguments.json");
  process.env.SDK_TEST_ARGUMENTS = record;
  assert.deepEqual(cleanProject({ binaryPath, projectPath: fixtureRoot, ports: [3010, 3011] }), []);
  assert.deepEqual(JSON.parse(fs.readFileSync(record, "utf8")), ["clean", "--dry-run", "--project", fixtureRoot, "--ports", "3010,3011", "--json"]);
  assert.deepEqual(await asyncCleanProject({ binaryPath, cwd: fixtureRoot }), []);
  assert.deepEqual(await asyncScanPorts([], { binaryPath }), []);
});

test("validates timeouts and output limits", () => {
  assert.throws(() => inspectPort(3000, { timeout: 0 }), RangeError);
  assert.throws(() => inspectPort(3000, { maxBuffer: -1 }), RangeError);
  assert.equal(DEFAULT_TIMEOUT, 30_000);
});

test("requires a boolean for destructive-operation mode", () => {
  assert.throws(() => freePort(3000, { dryRun: "false" }), TypeError);
});

test("reports a missing explicit CLI path with a typed error", () => {
  assert.throws(() => inspectPort(3000, { binaryPath: "" }), TypeError);
  assert.throws(
    () => inspectPort(3000, { binaryPath: "./missing-port-detective" }),
    PortDetectiveNotFoundError,
  );
});

test("async methods reject an already-aborted request before spawning", async () => {
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(asyncInspectPort(3000, {
    signal: controller.signal,
  }), { name: "AbortError" });
});

test("maps an async missing executable to the SDK error type", async () => {
  await assert.rejects(asyncInspectPort(3000, {
    binaryPath: "./missing-port-detective",
  }), PortDetectiveNotFoundError);
});

test("execution errors expose machine-readable process details", () => {
  const error = new PortDetectiveExecutionError("failed", 2, "stderr", "stdout");
  assert.equal(error.returncode, 2);
  assert.equal(error.stderr, "stderr");
  assert.equal(error.stdout, "stdout");
});

test("maps supported Node targets to platform package names", () => {
  assert.equal(platformPackageName("win32", "x64"), "port-detective-cli-win32-x64");
  assert.equal(platformPackageName("win32", "arm64"), "port-detective-cli-win32-arm64");
  assert.equal(platformPackageName("darwin", "x64"), "port-detective-cli-darwin-x64");
  assert.equal(platformPackageName("darwin", "arm64"), "port-detective-cli-darwin-arm64");
  assert.equal(platformPackageName("linux", "x64"), "port-detective-cli-linux-x64");
  assert.equal(platformPackageName("linux", "arm64"), "port-detective-cli-linux-arm64");
  assert.equal(platformPackageName("freebsd", "x64"), null);
  assert.equal(platformPackageName("win32", "ia32"), null);
  assert.equal(binaryRelativePath("win32"), "bin/port-detective.exe");
  assert.equal(binaryRelativePath("linux"), "bin/port-detective");
});
