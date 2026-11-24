"use strict";

const fs = require("node:fs");

const SUPPORTED_PLATFORMS = new Set(["win32", "darwin", "linux"]);
const SUPPORTED_ARCHITECTURES = new Set(["x64", "arm64"]);

function platformPackageName(platform = process.platform, arch = process.arch) {
  if (!SUPPORTED_PLATFORMS.has(platform) || !SUPPORTED_ARCHITECTURES.has(arch)) {
    return null;
  }
  return `port-detective-cli-${platform}-${arch}`;
}

function binaryRelativePath(platform = process.platform) {
  return platform === "win32" ? "bin/port-detective.exe" : "bin/port-detective";
}

function defaultBinaryName(platform = process.platform) {
  return platform === "win32" ? "port-detective.exe" : "port-detective";
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

module.exports = {
  platformPackageName,
  binaryRelativePath,
  defaultBinaryName,
  resolveBundledBinary,
};
