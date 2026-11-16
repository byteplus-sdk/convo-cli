// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

"use strict";

const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { healthy, recover, forwardSignal } = require("./run.js");

test("healthy requires a launchable binary with the expected version", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-run-test-"));
  const binary = path.join(dir, "convo-cli.exe");
  fs.writeFileSync(binary, "candidate");
  assert.equal(healthy(binary, "2.0.0", () => ({ status: 0, stdout: "convo-cli 2.0.0\n" })), true);
  assert.equal(healthy(binary, "2.0.0", () => ({ status: 0, stdout: "convo-cli 1.0.0\n" })), false);
  assert.equal(healthy(binary, "2.0.0", () => ({ error: new Error("bad image") })), false);
  fs.rmSync(dir, { recursive: true, force: true });
});

test("recover restores backup for a corrupt Windows candidate", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-run-test-"));
  const binary = path.join(dir, "convo-cli.exe");
  const backup = `${binary}.old`;
  fs.writeFileSync(binary, "broken");
  fs.writeFileSync(backup, "old");
  recover(binary, backup, "win32", "2.0.0", () => ({ status: 1 }));
  assert.equal(fs.readFileSync(binary, "utf8"), "old");
  assert.equal(fs.existsSync(backup), false);
  fs.rmSync(dir, { recursive: true, force: true });
});

test("recover preserves a verified Windows candidate and its backup", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-run-test-"));
  const binary = path.join(dir, "convo-cli.exe");
  const backup = `${binary}.old`;
  fs.writeFileSync(binary, "new");
  fs.writeFileSync(backup, "old");
  recover(binary, backup, "win32", "2.0.0", () => ({ status: 0, stdout: "convo-cli 2.0.0\n" }));
  assert.equal(fs.readFileSync(binary, "utf8"), "new");
  assert.equal(fs.readFileSync(backup, "utf8"), "old");
  fs.rmSync(dir, { recursive: true, force: true });
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  test(`npm launcher forwards ${signal} and waits for cleanup`, { skip: process.platform === "win32", timeout: 10000 }, async () => {
    const { spawn } = require("node:child_process");
    const { once } = require("node:events");
    const net = require("node:net");
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-signal-"));
    fs.mkdirSync(path.join(dir, "scripts", "bin"), { recursive: true });
    fs.writeFileSync(path.join(dir, "package.json"), JSON.stringify({ name: "@fixture/cli", version: "1.0.0", bin: { cli: "scripts/run.js" } }));
    fs.copyFileSync(path.join(__dirname, "run.js"), path.join(dir, "scripts", "run.js"));
    const binary = path.join(dir, "scripts", "bin", "cli");
    fs.writeFileSync(binary, `#!/usr/bin/env node
const net = require("node:net");
const fs = require("node:fs");
const server = net.createServer();
server.listen(0, "127.0.0.1", () => console.log(JSON.stringify({port: server.address().port, pid: process.pid})));
process.on(${JSON.stringify(signal)}, () => setTimeout(() => {
  fs.writeFileSync(${JSON.stringify(path.join(dir, "cleaned"))}, "yes");
  server.close(() => process.exit(0));
}, 150));
`, { mode: 0o755 });
    const child = spawn(process.execPath, [path.join(dir, "scripts", "run.js")], { detached: true });
    let timer;
    try {
      await Promise.race([
        (async () => {
          const closed = once(child, "close");
          const [raw] = await once(child.stdout, "data");
          const { port, pid } = JSON.parse(raw.toString());
          child.kill(signal);
          assert.deepEqual(await closed, [0, null]);
          assert.equal(fs.readFileSync(path.join(dir, "cleaned"), "utf8"), "yes");
          const probe = net.createServer();
          probe.listen(port, "127.0.0.1");
          await once(probe, "listening");
          await new Promise((resolve) => probe.close(resolve));
          assert.throws(() => process.kill(pid, 0), { code: "ESRCH" });
        })(),
        new Promise((_, reject) => {
          timer = setTimeout(() => reject(new Error("launcher cleanup timed out")), 5000);
        }),
      ]);
    } finally {
      clearTimeout(timer);
      try { process.kill(-child.pid, "SIGKILL"); } catch {}
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
}

test("npm launcher preserves a child failure exit code", { skip: process.platform === "win32" }, () => {
  const { spawnSync } = require("node:child_process");
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-exit-"));
  try {
    fs.mkdirSync(path.join(dir, "scripts", "bin"), { recursive: true });
    fs.writeFileSync(path.join(dir, "package.json"), JSON.stringify({ name: "@fixture/cli", version: "1.0.0", bin: { cli: "scripts/run.js" } }));
    fs.copyFileSync(path.join(__dirname, "run.js"), path.join(dir, "scripts", "run.js"));
    fs.writeFileSync(path.join(dir, "scripts", "bin", "cli"), "#!/bin/sh\nexit 7\n", { mode: 0o755 });
    assert.equal(spawnSync(process.execPath, [path.join(dir, "scripts", "run.js")], { timeout: 5000 }).status, 7);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("Windows console signals leave native shutdown to the child", () => {
 const signals = [];
 const child = { kill: (signal) => signals.push(signal) };
 forwardSignal(child, "SIGINT", "win32");
 forwardSignal(child, "SIGTERM", "win32");
 assert.deepEqual(signals, []);
 forwardSignal(child, "SIGINT", "linux");
 forwardSignal(child, "SIGTERM", "darwin");
 assert.deepEqual(signals, ["SIGINT", "SIGTERM"]);
});
