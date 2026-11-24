"use strict";

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

function abortError(signal) {
  if (signal.reason instanceof Error) return signal.reason;
  const error = new Error(signal.reason ? String(signal.reason) : "The operation was aborted");
  error.name = "AbortError";
  return error;
}

module.exports = {
  PortDetectiveError,
  PortDetectiveNotFoundError,
  PortDetectiveVersionError,
  PortDetectiveExecutionError,
  executionError,
  abortError,
};
