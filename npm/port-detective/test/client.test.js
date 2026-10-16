"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");

const {
  DEFAULT_TIMEOUT,
  PortDetectiveExecutionError,
  PortDetectiveNotFoundError,
  inspectPort,
  scanPorts,
  freePort,
  asyncInspectPort,
} = require("..");

test("validates ports before starting the CLI", () => {
  assert.throws(() => inspectPort(0), RangeError);
  assert.throws(() => scanPorts([3000, 65_536]), RangeError);
  assert.throws(() => freePort(true), RangeError);
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
