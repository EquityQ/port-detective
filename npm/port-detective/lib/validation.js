"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { resolveBundledBinary } = require("../platform");
const { PortDetectiveExecutionError } = require("./errors");

const DEFAULT_TIMEOUT = 30_000;
const DEFAULT_MAX_BUFFER = 10 * 1024 * 1024;
const VALID_PROTOCOLS = new Set(["tcp", "udp"]);
const VALID_PRESETS = new Set(["web", "python", "database", "project"]);
const VALID_WAIT_STATES = new Set(["open", "free"]);

function validatePort(port) {
  if (!Number.isInteger(port) || port < 1 || port > 65_535) {
    throw new RangeError(`port must be an integer between 1 and 65535, got ${port}`);
  }
}

function validatePorts(ports) {
  if (!Array.isArray(ports)) {
    throw new TypeError("ports must be an array of integers");
  }
  ports.forEach(validatePort);
}

function validateProtocol(protocol = "tcp") {
  if (typeof protocol !== "string") {
    throw new TypeError(`protocol must be "tcp" or "udp", got ${protocol}`);
  }
  const normalized = protocol.trim().toLowerCase();
  if (!VALID_PROTOCOLS.has(normalized)) {
    throw new RangeError(`protocol must be "tcp" or "udp", got ${protocol}`);
  }
  return normalized;
}

function validatePreviewId(previewId) {
  if (previewId === undefined) return null;
  if (typeof previewId !== "string" || previewId.trim().length === 0) {
    throw new TypeError("previewId must be a non-empty string");
  }
  return previewId;
}

function validatePreset(preset) {
  if (preset === undefined) return null;
  if (typeof preset !== "string" || !VALID_PRESETS.has(preset)) {
    throw new RangeError(`preset must be one of web, python, database, or project; got ${preset}`);
  }
  return preset;
}

function formatDurationMs(ms, label) {
  if (typeof ms !== "number" || !Number.isFinite(ms) || ms <= 0) {
    throw new RangeError(`${label} must be a positive number of milliseconds, got ${ms}`);
  }
  return `${Math.max(1, Math.round(ms))}ms`;
}

// An omitted selection (undefined/null) means "use the documented default"; an
// explicitly empty array means "select nothing" and must never be forwarded as an
// omitted --ports flag, which would silently widen the scope to the default set or
// to every project listener. Returns a comma-joined list, or null for the default.
function portSelection(ports, label) {
  if (ports === undefined || ports === null) return null;
  validatePorts(ports);
  if (ports.length === 0) {
    throw new TypeError(
      `${label} must not be an empty array; omit it or pass undefined/null to use the default selection`,
    );
  }
  return ports.join(",");
}

function validateOptions(options, resolveBinary, asyncMode = false) {
  if (options === undefined) options = {};
  if (options === null || typeof options !== "object" || Array.isArray(options)) {
    throw new TypeError("options must be an object");
  }

  const timeout = options.timeout === undefined ? DEFAULT_TIMEOUT : options.timeout;
  if (timeout !== null && (!Number.isFinite(timeout) || timeout <= 0)) {
    throw new RangeError("timeout must be a positive number of milliseconds or null");
  }
  const maxBuffer = options.maxBuffer === undefined ? DEFAULT_MAX_BUFFER : options.maxBuffer;
  if (!Number.isSafeInteger(maxBuffer) || maxBuffer < 1) {
    throw new RangeError("maxBuffer must be a positive safe integer");
  }

  let cwd;
  if (options.cwd !== undefined) {
    if (typeof options.cwd !== "string" || options.cwd.length === 0) {
      throw new TypeError("cwd must be a non-empty directory path");
    }
    cwd = path.resolve(options.cwd);
    try {
      if (!fs.statSync(cwd).isDirectory()) throw new Error("not a directory");
    } catch (error) {
      throw new TypeError(`cwd is not an accessible directory: ${options.cwd}`, { cause: error });
    }
  }

  if (asyncMode && options.signal !== undefined &&
      (typeof options.signal.addEventListener !== "function" || typeof options.signal.aborted !== "boolean")) {
    throw new TypeError("signal must be an AbortSignal");
  }

  const configured = options.binaryPath || process.env.PORT_DETECTIVE_BIN;
  const binary = resolveBinary(options.binaryPath);
  const bundled = configured ? null : resolveBundledBinary();
  return {
    binary,
    trustedVersion: bundled !== null,
    cwd,
    timeout,
    maxBuffer,
    signal: asyncMode ? options.signal : undefined,
  };
}

function appendProjectArgs(args, options) {
  if (options.projectPath !== undefined) {
    if (typeof options.projectPath !== "string" || options.projectPath.length === 0) {
      throw new TypeError("projectPath must be a non-empty directory path");
    }
    args.push("--project", options.projectPath);
  }
  const selection = portSelection(options.ports, "options.ports");
  if (selection !== null) args.push("--ports", selection);
  const previewId = validatePreviewId(options.previewId);
  if (previewId !== null) args.push("--preview-id", previewId);
}

function validateWaitState(state = "open") {
  if (typeof state !== "string" || !VALID_WAIT_STATES.has(state)) {
    throw new RangeError(`state must be "open" or "free", got ${state}`);
  }
  return state;
}

function requireObject(value, command) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new PortDetectiveExecutionError(`expected an object response for ${command}`, 0);
  }
  return value;
}

function requireArray(value, command) {
  if (value === null) return [];
  if (!Array.isArray(value)) {
    throw new PortDetectiveExecutionError(`expected an array response for ${command}`, 0);
  }
  return value;
}

module.exports = {
  DEFAULT_TIMEOUT,
  DEFAULT_MAX_BUFFER,
  validatePort,
  validatePorts,
  validateProtocol,
  validatePreviewId,
  validatePreset,
  validateWaitState,
  formatDurationMs,
  portSelection,
  validateOptions,
  appendProjectArgs,
  requireObject,
  requireArray,
};
