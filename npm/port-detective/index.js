"use strict";

const {
  PortDetectiveError,
  PortDetectiveNotFoundError,
  PortDetectiveVersionError,
  PortDetectiveExecutionError,
} = require("./lib/errors");
const {
  DEFAULT_TIMEOUT,
  validatePort,
  validateProtocol,
  validatePreviewId,
  validatePreset,
  validateWaitState,
  formatDurationMs,
  portSelection,
  appendProjectArgs,
  requireObject,
  requireArray,
} = require("./lib/validation");
const { runCliSync, runCliAsync } = require("./lib/runner");

function checkArgs(port, options) {
  validatePort(port);
  const proto = validateProtocol(options.protocol);
  const args = ["check", String(port)];
  if (proto !== "tcp") args.push("--protocol", proto);
  return args;
}

function scanArgs(ports, options) {
  const proto = validateProtocol(options.protocol);
  const preset = validatePreset(options.preset);
  const selection = portSelection(ports, "ports");
  if (selection !== null && preset !== null) {
    throw new TypeError("ports and options.preset cannot be used together");
  }
  if (options.projectPath !== undefined && preset !== "project") {
    throw new TypeError("options.projectPath requires options.preset === 'project'");
  }
  const args = ["scan"];
  if (selection !== null) args.push("--ports", selection);
  if (preset !== null) args.push("--preset", preset);
  if (options.projectPath !== undefined) {
    if (typeof options.projectPath !== "string" || options.projectPath.length === 0) {
      throw new TypeError("projectPath must be a non-empty directory path");
    }
    args.push("--project", options.projectPath);
  }
  if (proto !== "tcp") args.push("--protocol", proto);
  return args;
}

function freeArgs(port, options) {
  validatePort(port);
  if (options.dryRun !== undefined && typeof options.dryRun !== "boolean") {
    throw new TypeError("dryRun must be a boolean");
  }
  const args = ["kill", String(port), options.dryRun === false ? "--yes" : "--dry-run"];
  const previewId = validatePreviewId(options.previewId);
  if (previewId !== null) args.push("--preview-id", previewId);
  return args;
}

function cleanArgs(options) {
  if (options.dryRun !== undefined && typeof options.dryRun !== "boolean") {
    throw new TypeError("dryRun must be a boolean");
  }
  const args = ["clean", options.dryRun === false ? "--yes" : "--dry-run"];
  appendProjectArgs(args, options);
  return args;
}

function waitArgs(port, options) {
  validatePort(port);
  const state = validateWaitState(options.state);
  const waitTimeout = options.waitTimeout === undefined ? 30_000 : options.waitTimeout;
  const interval = options.interval === undefined ? 250 : options.interval;
  const args = [
    "wait",
    String(port),
    "--state",
    state,
    "--timeout",
    formatDurationMs(waitTimeout, "waitTimeout"),
    "--interval",
    formatDurationMs(interval, "interval"),
  ];
  const cliOptions = options.timeout === undefined ? { ...options, timeout: waitTimeout + 5_000 } : options;
  return { args, cliOptions };
}

function findFreeArgs(options) {
  const from = options.from === undefined ? 3000 : options.from;
  const to = options.to === undefined ? 3100 : options.to;
  validatePort(from);
  validatePort(to);
  if (from > to) {
    throw new RangeError(`from (${from}) must be <= to (${to})`);
  }
  return ["find-free", "--from", String(from), "--to", String(to)];
}

function inspectPort(port, options = {}) {
  return requireObject(runCliSync(checkArgs(port, options), options), "check");
}

function scanPorts(ports, options = {}) {
  return requireArray(runCliSync(scanArgs(ports, options), options), "scan");
}

function freePort(port, options = {}) {
  return requireObject(runCliSync(freeArgs(port, options), options), "kill");
}

function cleanProject(options = {}) {
  return requireArray(runCliSync(cleanArgs(options), options), "clean");
}

function waitForPort(port, options = {}) {
  const { args, cliOptions } = waitArgs(port, options);
  return requireObject(runCliSync(args, cliOptions), "wait");
}

function findFreePort(options = {}) {
  return requireObject(runCliSync(findFreeArgs(options), options), "find-free");
}

function doctor(options = {}) {
  return requireObject(runCliSync(["doctor"], options, { allowExitCodes: [0, 5] }), "doctor");
}

async function asyncInspectPort(port, options = {}) {
  return requireObject(await runCliAsync(checkArgs(port, options), options), "check");
}

async function asyncScanPorts(ports, options = {}) {
  return requireArray(await runCliAsync(scanArgs(ports, options), options), "scan");
}

async function asyncFreePort(port, options = {}) {
  return requireObject(await runCliAsync(freeArgs(port, options), options), "kill");
}

async function asyncCleanProject(options = {}) {
  return requireArray(await runCliAsync(cleanArgs(options), options), "clean");
}

async function asyncWaitForPort(port, options = {}) {
  const { args, cliOptions } = waitArgs(port, options);
  return requireObject(await runCliAsync(args, cliOptions), "wait");
}

async function asyncFindFreePort(options = {}) {
  return requireObject(await runCliAsync(findFreeArgs(options), options), "find-free");
}

async function asyncDoctor(options = {}) {
  return requireObject(await runCliAsync(["doctor"], options, { allowExitCodes: [0, 5] }), "doctor");
}

module.exports = {
  DEFAULT_TIMEOUT,
  PortDetectiveError,
  PortDetectiveNotFoundError,
  PortDetectiveVersionError,
  PortDetectiveExecutionError,
  inspectPort,
  scanPorts,
  freePort,
  cleanProject,
  waitForPort,
  findFreePort,
  doctor,
  asyncInspectPort,
  asyncScanPorts,
  asyncFreePort,
  asyncCleanProject,
  asyncWaitForPort,
  asyncFindFreePort,
  asyncDoctor,
};
