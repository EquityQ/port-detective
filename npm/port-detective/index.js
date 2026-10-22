"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawn, spawnSync } = require("node:child_process");
const { binaryRelativePath, platformPackageName } = require("./platform");

const DEFAULT_TIMEOUT = 30_000;
const DEFAULT_MAX_BUFFER = 10 * 1024 * 1024;
const SDK_VERSION = require("./package.json").version;
const validatedBinaries = new Map();

class PortDetectiveError extends Error {
  constructor(message, options = {}) {
    super(message, options);
    this.name = new.target.name;
  }
}

class PortDetectiveNotFoundError extends PortDetectiveError {}
class PortDetectiveVersionError extends PortDetectiveError {}

class PortDetectiveExecutionError extends PortDetectiveError {
  constructor(message, returncode, stderr = "", stdout = "", options = {}) {
    super(message, options);
    this.returncode = returncode;
    this.stderr = stderr;
    this.stdout = stdout;
  }
}

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

function resolveBundledBinary() {
  const packageName = platformPackageName();
  if (!packageName) return null;
  const relativePath = binaryRelativePath();
  try {
    const candidate = require.resolve(`${packageName}/${relativePath}`);
    if (fs.statSync(candidate).isFile()) return candidate;
  } catch {
    // The optional package is absent when using a source checkout or an unsupported release.
  }
  return null;
}

function validateOptions(options, asyncMode = false) {
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

function resolveBinary(binaryPath) {
  if (binaryPath !== undefined && (typeof binaryPath !== "string" || binaryPath.length === 0)) {
    throw new TypeError("binaryPath must be a non-empty path or command name");
  }
  const configured = binaryPath || process.env.PORT_DETECTIVE_BIN;
  const bundled = configured ? null : resolveBundledBinary();
  const candidate = configured || bundled || (process.platform === "win32" ? "port-detective.exe" : "port-detective");

  if (typeof candidate !== "string" || candidate.length === 0) {
    throw new TypeError("binaryPath must be a non-empty path or command name");
  }

  const hasPath = path.isAbsolute(candidate) || candidate.includes("/") || candidate.includes("\\");
  if (hasPath) {
    const resolved = path.resolve(candidate);
    try {
      if (fs.statSync(resolved).isFile()) return resolved;
    } catch {
      // The caller receives a consistent not-found error below.
    }
    throw new PortDetectiveNotFoundError(`port-detective executable not found: ${candidate}`);
  }

  if (configured) {
    const localCandidate = path.resolve(candidate);
    try {
      if (fs.statSync(localCandidate).isFile()) return localCandidate;
    } catch {
      // Bare command names are resolved through PATH by the operating system.
    }
  }
  const directories = (process.env.PATH || process.env.Path || "").split(path.delimiter);
  for (const directory of directories) {
    const executable = path.resolve(directory, candidate);
    try {
      if (fs.statSync(executable).isFile()) {
        fs.accessSync(executable, fs.constants.X_OK);
        return executable;
      }
    } catch {
      // Try the next PATH entry; spawn reports a typed not-found error if absent.
    }
  }
  return candidate;
}

function binaryIdentity(binary) {
  try {
    const info = fs.statSync(binary, { bigint: true });
    return [info.dev, info.ino, info.size, info.mtimeNs, info.ctimeNs].join(":");
  } catch {
    return null;
  }
}

function versionCached(config) {
  const identity = binaryIdentity(config.binary);
  return config.trustedVersion || (identity !== null && validatedBinaries.get(config.binary) === identity);
}

function acceptVersion(config, result, identity) {
  const output = result.stdout.trim();
  const match = /^(?:port-detective(?:\.exe)? version )?v?(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?$/.exec(output);
  const expected = SDK_VERSION.split(".").slice(0, 2).join(".");
  if (!match || `${match[1]}.${match[2]}` !== expected) {
    throw new PortDetectiveVersionError(
      `Incompatible port-detective CLI at ${config.binary}: ${JSON.stringify(output)}. ` +
      `This SDK requires CLI ${expected}.x (including versioned development builds). ` +
      "Upgrade the CLI or set PORT_DETECTIVE_BIN to a compatible binary.",
    );
  }
  const current = binaryIdentity(config.binary);
  if (identity !== null && current !== identity) {
    throw new PortDetectiveVersionError(`CLI changed during its version check: ${config.binary}; retry the call`);
  }
  if (current !== null) validatedBinaries.set(config.binary, current);
}

function executionError(error, result = {}) {
  if (error && error.code === "ENOENT") {
    return new PortDetectiveNotFoundError(
      `port-detective executable not found; install the matching CLI package or set PORT_DETECTIVE_BIN: ${error.message}`,
      { cause: error },
    );
  }
  const timedOut = error && error.code === "ETIMEDOUT";
  const message = timedOut ? "port-detective command timed out" :
    (error ? `failed to execute port-detective: ${error.message}` :
      (result.stderr.trim() || `CLI returned code ${result.status}`));
  return new PortDetectiveExecutionError(
    message,
    result.status === null || result.status === undefined ? -1 : result.status,
    result.stderr || "",
    result.stdout || "",
    error ? { cause: error } : {},
  );
}

function decodeResponse(stdout, stderr, returncode) {
  const output = stdout.trim();
  if (output.length === 0) return null;
  try {
    return JSON.parse(output);
  } catch (error) {
    throw new PortDetectiveExecutionError(
      `invalid JSON returned by port-detective: ${error.message}`,
      returncode,
      stderr,
      stdout,
      { cause: error },
    );
  }
}

function commandArgs(args) {
  return [...args, "--json"];
}

function runCliSync(args, options) {
  const config = validateOptions(options);
  const deadline = config.timeout === null ? null : Date.now() + config.timeout;
  if (!versionCached(config)) {
    const identity = binaryIdentity(config.binary);
    acceptVersion(config, executeSync(["--version"], config), identity);
  }
  if (deadline !== null) {
    config.timeout = deadline - Date.now();
    if (config.timeout <= 0) throw new PortDetectiveExecutionError("port-detective command timed out", -1);
  }
  const result = executeSync(commandArgs(args), config);
  return decodeResponse(result.stdout, result.stderr, result.status);
}

function executeSync(args, config) {
  const result = spawnSync(config.binary, args, {
    cwd: config.cwd,
    encoding: "utf8",
    windowsHide: true,
    timeout: config.timeout === null ? undefined : config.timeout,
    maxBuffer: config.maxBuffer,
  });
  const stdout = result.stdout || "";
  const stderr = result.stderr || "";

  if (result.error) throw executionError(result.error, { ...result, stdout, stderr });
  if (result.status !== 0) throw executionError(null, { ...result, stdout, stderr });
  return { stdout, stderr, status: result.status };
}

function abortError(signal) {
  if (signal.reason instanceof Error) return signal.reason;
  const error = new Error(signal.reason ? String(signal.reason) : "The operation was aborted");
  error.name = "AbortError";
  return error;
}

async function runCliAsync(args, options) {
  const config = validateOptions(options, true);
  if (config.signal && config.signal.aborted) throw abortError(config.signal);
  const deadline = config.timeout === null ? null : Date.now() + config.timeout;
  if (!versionCached(config)) {
    const identity = binaryIdentity(config.binary);
    acceptVersion(config, await executeAsync(["--version"], config), identity);
  }
  if (config.signal && config.signal.aborted) throw abortError(config.signal);
  if (deadline !== null) {
    config.timeout = deadline - Date.now();
    if (config.timeout <= 0) throw new PortDetectiveExecutionError("port-detective command timed out", -1);
  }
  const result = await executeAsync(commandArgs(args), config);
  return decodeResponse(result.stdout, result.stderr, result.code);
}

function executeAsync(args, config) {
  return new Promise((resolve, reject) => {
    let child;
    try {
      child = spawn(config.binary, args, {
        cwd: config.cwd,
        windowsHide: true,
        stdio: ["ignore", "pipe", "pipe"],
      });
    } catch (error) {
      reject(executionError(error));
      return;
    }

    let stdout = "";
    let stderr = "";
    let timedOut = false;
    let exceededBuffer = false;
    let settled = false;
    let timer = null;
    const cleanup = () => {
      if (timer) clearTimeout(timer);
      if (config.signal) config.signal.removeEventListener("abort", onAbort);
    };
    const finish = (error, value) => {
      if (settled) return;
      settled = true;
      cleanup();
      if (error) reject(error);
      else resolve(value);
    };
    const onAbort = () => child.kill();
    const collect = (stream, which) => {
      let current = "";
      stream.setEncoding("utf8");
      stream.on("data", (chunk) => {
        if (exceededBuffer) return;
        const next = current + chunk;
        if (Buffer.byteLength(next, "utf8") > config.maxBuffer) {
          exceededBuffer = true;
          child.kill();
          return;
        }
        current = next;
        if (which === "stdout") stdout = current;
        else stderr = current;
      });
    };

    collect(child.stdout, "stdout");
    collect(child.stderr, "stderr");

    timer = config.timeout === null ? null : setTimeout(() => {
      timedOut = true;
      child.kill();
    }, config.timeout);
    if (config.signal) config.signal.addEventListener("abort", onAbort, { once: true });

    child.on("error", (error) => finish(executionError(error, { stdout, stderr })));
    child.on("close", (code) => {
      if (config.signal && config.signal.aborted) {
        finish(abortError(config.signal));
      } else if (timedOut) {
        finish(new PortDetectiveExecutionError("port-detective command timed out", -1, stderr, stdout));
      } else if (exceededBuffer) {
        finish(new PortDetectiveExecutionError("port-detective output exceeded maxBuffer", -1, stderr, stdout));
      } else if (code !== 0) {
        finish(executionError(null, { status: code, stdout, stderr }));
      } else {
        finish(null, { stdout, stderr, code });
      }
    });
  });
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

function inspectPort(port, options = {}) {
  validatePort(port);
  return requireObject(runCliSync(["check", String(port)], options), "check");
}

function scanPorts(ports, options = {}) {
  let args = ["scan"];
  if (ports !== undefined && ports !== null && ports.length > 0) {
    validatePorts(ports);
    args = [...args, "--ports", ports.join(",")];
  } else if (ports !== undefined && ports !== null) {
    validatePorts(ports);
  }
  return requireArray(runCliSync(args, options), "scan");
}

function freePort(port, options = {}) {
  validatePort(port);
  if (options.dryRun !== undefined && typeof options.dryRun !== "boolean") {
    throw new TypeError("dryRun must be a boolean");
  }
  const dryRun = options.dryRun !== false;
  const args = ["kill", String(port)];
  if (dryRun) args.push("--dry-run");
  else args.push("--yes");
  return requireObject(runCliSync(args, options), "kill");
}

function cleanProject(options = {}) {
  if (options.dryRun !== undefined && typeof options.dryRun !== "boolean") {
    throw new TypeError("dryRun must be a boolean");
  }
  const args = ["clean", options.dryRun === false ? "--yes" : "--dry-run"];
  appendProjectArgs(args, options);
  return requireArray(runCliSync(args, options), "clean");
}

async function asyncInspectPort(port, options = {}) {
  validatePort(port);
  return requireObject(await runCliAsync(["check", String(port)], options), "check");
}

async function asyncScanPorts(ports, options = {}) {
  let args = ["scan"];
  if (ports !== undefined && ports !== null && ports.length > 0) {
    validatePorts(ports);
    args = [...args, "--ports", ports.join(",")];
  } else if (ports !== undefined && ports !== null) {
    validatePorts(ports);
  }
  return requireArray(await runCliAsync(args, options), "scan");
}

async function asyncFreePort(port, options = {}) {
  validatePort(port);
  if (options.dryRun !== undefined && typeof options.dryRun !== "boolean") {
    throw new TypeError("dryRun must be a boolean");
  }
  const args = ["kill", String(port), options.dryRun === false ? "--yes" : "--dry-run"];
  return requireObject(await runCliAsync(args, options), "kill");
}

async function asyncCleanProject(options = {}) {
  if (options.dryRun !== undefined && typeof options.dryRun !== "boolean") {
    throw new TypeError("dryRun must be a boolean");
  }
  const args = ["clean", options.dryRun === false ? "--yes" : "--dry-run"];
  appendProjectArgs(args, options);
  return requireArray(await runCliAsync(args, options), "clean");
}

function appendProjectArgs(args, options) {
  if (options.projectPath !== undefined) {
    if (typeof options.projectPath !== "string" || options.projectPath.length === 0) {
      throw new TypeError("projectPath must be a non-empty directory path");
    }
    args.push("--project", options.projectPath);
  }
  if (options.ports !== undefined && options.ports !== null) {
    validatePorts(options.ports);
    if (options.ports.length > 0) args.push("--ports", options.ports.join(","));
  }
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
  asyncInspectPort,
  asyncScanPorts,
  asyncFreePort,
  asyncCleanProject,
};
