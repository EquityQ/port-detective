#!/usr/bin/env node

"use strict";

const { spawnSync } = require("node:child_process");
const { binaryRelativePath, platformPackageName } = require("./platform");

function resolveCli() {
  if (process.env.PORT_DETECTIVE_BIN) return process.env.PORT_DETECTIVE_BIN;
  const packageName = platformPackageName();
  if (packageName) {
    try {
      return require.resolve(`${packageName}/${binaryRelativePath()}`);
    } catch {
      // Fall back to a separately installed CLI or PATH.
    }
  }
  return process.platform === "win32" ? "port-detective.exe" : "port-detective";
}

const result = spawnSync(resolveCli(), process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
});

if (result.error) {
  console.error(`failed to execute port-detective: ${result.error.message}`);
  process.exit(1);
}
process.exit(result.status === null ? 1 : result.status);
