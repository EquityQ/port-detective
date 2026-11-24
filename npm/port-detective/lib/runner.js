"use strict";

const fs = require("node:fs");
const path = require("node:path");
const { spawn, spawnSync } = require("node:child_process");
const { defaultBinaryName, resolveBundledBinary } = require("../platform");
const {
  PortDetectiveNotFoundError,
  PortDetectiveVersionError,
  PortDetectiveExecutionError,
  executionError,
  abortError,
} = require("./errors");
const { validateOptions } = require("./validation");

const SDK_VERSION = require("../package.json").version;
const validatedBinaries = new Map();

function resolveBinary(binaryPath) {
  if (binaryPath !== undefined && (typeof binaryPath !== "string" || binaryPath.length === 0)) {
    throw new TypeError("binaryPath must be a non-empty path or command name");
  }
  const configured = binaryPath || process.env.PORT_DETECTIVE_BIN;
  const bundled = configured ? null : resolveBundledBinary();
  const candidate = configured || bundled || defaultBinaryName();

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

function runCliSync(args, options, runOptions = {}) {
  const allowExitCodes = runOptions.allowExitCodes || [0];
  const config = validateOptions(options, resolveBinary);
  const deadline = config.timeout === null ? null : Date.now() + config.timeout;
  if (!versionCached(config)) {
    const identity = binaryIdentity(config.binary);
    acceptVersion(config, executeSync(["--version"], config, [0]), identity);
  }
  if (deadline !== null) {
    config.timeout = deadline - Date.now();
    if (config.timeout <= 0) throw new PortDetectiveExecutionError("port-detective command timed out", -1);
  }
  const result = executeSync(commandArgs(args), config, allowExitCodes);
  return decodeResponse(result.stdout, result.stderr, result.status);
}

function executeSync(args, config, allowExitCodes = [0]) {
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
  if (!allowExitCodes.includes(result.status)) throw executionError(null, { ...result, stdout, stderr });
  return { stdout, stderr, status: result.status };
}

async function runCliAsync(args, options, runOptions = {}) {
  const allowExitCodes = runOptions.allowExitCodes || [0];
  const config = validateOptions(options, resolveBinary, true);
  if (config.signal && config.signal.aborted) throw abortError(config.signal);
  const deadline = config.timeout === null ? null : Date.now() + config.timeout;
  if (!versionCached(config)) {
    const identity = binaryIdentity(config.binary);
    acceptVersion(config, await executeAsync(["--version"], config, [0]), identity);
  }
  if (config.signal && config.signal.aborted) throw abortError(config.signal);
  if (deadline !== null) {
    config.timeout = deadline - Date.now();
    if (config.timeout <= 0) throw new PortDetectiveExecutionError("port-detective command timed out", -1);
  }
  const result = await executeAsync(commandArgs(args), config, allowExitCodes);
  return decodeResponse(result.stdout, result.stderr, result.code);
}

function executeAsync(args, config, allowExitCodes = [0]) {
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
      } else if (!allowExitCodes.includes(code)) {
        finish(executionError(null, { status: code, stdout, stderr }));
      } else {
        finish(null, { stdout, stderr, code });
      }
    });
  });
}

module.exports = {
  runCliSync,
  runCliAsync,
};
