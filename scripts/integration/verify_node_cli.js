"use strict";

const assert = require("node:assert/strict");
const net = require("node:net");
const clientPath = process.env.PORT_DETECTIVE_NODE_MODULE || "../../npm/port-detective";
const {
  asyncInspectPort,
  asyncCleanProject,
  asyncFreePort,
  asyncScanPorts,
  cleanProject,
  freePort,
  inspectPort,
  scanPorts,
} = require(clientPath);

async function main() {
  const listener = net.createServer();
  await new Promise((resolve, reject) => {
    listener.once("error", reject);
    listener.listen(0, "127.0.0.1", resolve);
  });

  const port = listener.address().port;
  try {
    const diagnostic = inspectPort(port);
    assert.equal(diagnostic.port, port);
    assert.equal(diagnostic.is_open, true);
    assert.equal(diagnostic.protocol, "tcp");
    assert.ok(diagnostic.pid > 0);

    const scan = scanPorts([port]);
    assert.equal(scan.length, 1);
    assert.equal(scan[0].port, port);
    assert.equal(scan[0].pid, diagnostic.pid);

    const preview = freePort(port);
    assert.equal(preview.dry_run, true);
    assert.equal(preview.diagnostic.pid, diagnostic.pid);

    const asyncDiagnostic = await asyncInspectPort(port);
    assert.equal(asyncDiagnostic.is_open, true);
    assert.equal(asyncDiagnostic.pid, diagnostic.pid);

    const asyncScan = await asyncScanPorts([port]);
    assert.equal(asyncScan.length, 1);
    assert.equal(asyncScan[0].pid, diagnostic.pid);

    const asyncPreview = await asyncFreePort(port);
    assert.equal(asyncPreview.dry_run, true);
    assert.equal(asyncPreview.diagnostic.pid, diagnostic.pid);

    const cleanPreview = cleanProject({ cwd: process.cwd() });
    assert.ok(Array.isArray(cleanPreview));
    const asyncCleanPreview = await asyncCleanProject({ cwd: process.cwd() });
    assert.ok(Array.isArray(asyncCleanPreview));
    console.log(`verified Node.js SDK against CLI for TCP port ${port}`);
  } finally {
    await new Promise((resolve, reject) => {
      listener.close((error) => error ? reject(error) : resolve());
    });
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
