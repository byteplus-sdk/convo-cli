#!/usr/bin/env node
// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

"use strict";

const fs = require("fs");
const path = require("path");
const { spawn, spawnSync } = require("child_process");

const pkg = require(path.join(__dirname, "..", "package.json"));
const entries = Object.entries(pkg.bin || {});
if (entries.length !== 1 || entries[0][1] !== "scripts/run.js") throw new Error("invalid package executable metadata");
const packageName = pkg.name;
const binaryName = entries[0][0];
const executable = process.platform === "win32" ? `${binaryName}.exe` : binaryName;
const binary = path.join(__dirname, "bin", executable);
const stale = `${binary}.old`;
const expectedVersion = pkg.version;

function healthy(candidate, version = expectedVersion, spawn = spawnSync) {
  if (!fs.existsSync(candidate)) return false;
  const result = spawn(candidate, ["--version"], { encoding: "utf8" });
  if (result.error || result.status !== 0) return false;
  const fields = String(result.stdout || "").trim().split(/\s+/);
  return fields.length >= 2 && fields[1].replace(/^v/, "") === version.replace(/^v/, "");
}

function recover(candidate = binary, backup = stale, platform = process.platform, version = expectedVersion, spawn = spawnSync) {
  if (platform !== "win32" || !fs.existsSync(backup)) return;
  if (healthy(candidate, version, spawn)) return;
  fs.rmSync(candidate, { force: true });
  fs.renameSync(backup, candidate);
}

function forwardSignal(child, signal, platform = process.platform) {
  if (platform !== "win32") child.kill(signal);
}

async function main() {
  recover();
  if (!fs.existsSync(binary)) {
    console.error(`${binaryName} binary is missing; reinstall with: npm install -g ${packageName}`);
    return 1;
  }

  return new Promise((resolve) => {
    const child = spawn(binary, process.argv.slice(2), { stdio: "inherit" });
    const forward = (signal) => forwardSignal(child, signal);
    const onInterrupt = () => forward("SIGINT");
    const onTerminate = () => forward("SIGTERM");
    process.on("SIGINT", onInterrupt);
    process.on("SIGTERM", onTerminate);
    const cleanup = () => {
      process.removeListener("SIGINT", onInterrupt);
      process.removeListener("SIGTERM", onTerminate);
    };
    child.once("error", (error) => {
      cleanup();
      if (process.platform === "win32" && fs.existsSync(stale)) {
        fs.rmSync(binary, { force: true });
        fs.renameSync(stale, binary);
      }
      console.error(`failed to run ${binaryName}: ${error.message}`);
      resolve(1);
    });
    child.once("close", (code, signal) => {
      cleanup();
      if (process.platform === "win32" && fs.existsSync(stale)) fs.rmSync(stale, { force: true });
      resolve(code === null ? 128 + (require("os").constants.signals[signal] || 1) : code);
    });
  });
}

if (require.main === module) main().then((code) => { process.exitCode = code; });

module.exports = { healthy, recover, forwardSignal };
