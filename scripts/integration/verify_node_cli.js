"use strict";

const assert = require("node:assert/strict");
const net = require("node:net");
const clientPath = process.env.PORT_DETECTIVE_NODE_MODULE || "../../npm/port-detective";
const {
  asyncDoctor,
  asyncFindFreePort,
  asyncInspectPort,
  asyncCleanProject,
  asyncCleanProjectResult,
  asyncFreePort,
  asyncScanPorts,
  asyncWaitForPort,
  cleanProject,
  cleanProjectResult,
  doctor,
  findFreePort,
  freePort,
  inspectPort,
  scanPorts,
  waitForPort,
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

    const cleanEnvelope = cleanProjectResult({ cwd: process.cwd() });
    assert.equal(cleanEnvelope.dry_run, true);
    assert.ok(Array.isArray(cleanEnvelope.cleaned));
    assert.equal(cleanEnvelope.count, cleanEnvelope.cleaned.length);
    assert.ok(typeof cleanEnvelope.cwd === "string" && cleanEnvelope.cwd.length > 0);
    assert.ok(typeof cleanEnvelope.scope === "string" && cleanEnvelope.scope.length > 0);
    if (cleanEnvelope.count > 0) {
      assert.ok(typeof cleanEnvelope.preview_id === "string" && cleanEnvelope.preview_id.length > 0);
    }

    const asyncCleanEnvelope = await asyncCleanProjectResult({ cwd: process.cwd() });
    assert.equal(asyncCleanEnvelope.dry_run, true);
    assert.ok(Array.isArray(asyncCleanEnvelope.cleaned));
    assert.equal(asyncCleanEnvelope.count, asyncCleanEnvelope.cleaned.length);
    assert.ok(typeof asyncCleanEnvelope.cwd === "string" && asyncCleanEnvelope.cwd.length > 0);
    assert.ok(typeof asyncCleanEnvelope.scope === "string" && asyncCleanEnvelope.scope.length > 0);
    if (asyncCleanEnvelope.count > 0) {
      assert.ok(typeof asyncCleanEnvelope.preview_id === "string" && asyncCleanEnvelope.preview_id.length > 0);
    }

    assert.ok(Array.isArray(diagnostic.bind_addresses) && diagnostic.bind_addresses.length > 0);
    assert.ok(typeof preview.preview_id === "string" && preview.preview_id.length > 0);

    const waited = waitForPort(port, { state: "open", waitTimeout: 2000, interval: 50 });
    assert.equal(waited.is_open, true);
    const asyncWaited = await asyncWaitForPort(port, { state: "open", waitTimeout: 2000, interval: 50 });
    assert.equal(asyncWaited.is_open, true);

    const free = findFreePort({ from: 45101, to: 45200 });
    assert.ok(free.port >= 45101 && free.port <= 45200);
    const asyncFree = await asyncFindFreePort({ from: 45101, to: 45200 });
    assert.ok(asyncFree.port >= 45101 && asyncFree.port <= 45200);

    const report = doctor();
    assert.ok(report.status === "ok" || report.status === "degraded");
    const asyncReport = await asyncDoctor();
    assert.ok(asyncReport.status === "ok" || asyncReport.status === "degraded");
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
