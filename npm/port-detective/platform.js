"use strict";

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

module.exports = { platformPackageName, binaryRelativePath };
