// Copyright (c) 2026 BytePlus Pte. Ltd.
// SPDX-License-Identifier: MIT

"use strict";

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const test = require("node:test");
const { archiveName, expectedChecksum, isAllowedURL, packageIdentity, readBundledArchive, replaceBinary, target, urls, verify } = require("./install.js");

test("maps supported release targets", () => {
  assert.deepEqual(target("darwin", "arm64"), { os: "darwin", arch: "arm64" });
  assert.deepEqual(target("linux", "x64"), { os: "linux", arch: "amd64" });
  assert.equal(archiveName("1.2.3", "win32", "x64"), "convo-cli_1.2.3_windows_amd64.zip");
  assert.throws(() => target("freebsd", "x64"), /unsupported platform/);
});

test("uses an alternate package identity", () => {
  const packageName = "@example/convo-cli";
  const identity = packageIdentity({ name: packageName, bin: { "convo-cli": "scripts/run.js" } });
  assert.deepEqual(identity, { packageName, binaryName: "convo-cli" });
  assert.equal(archiveName("1.2.3", "linux", "arm64", identity), "convo-cli_1.2.3_linux_arm64.tar.gz");
});

test("accepts only approved HTTPS hosts", () => {
  assert.equal(isAllowedURL("https://github.com/byteplus-sdk/release"), true);
  assert.equal(isAllowedURL("http://github.com/byteplus-sdk/release"), false);
  assert.equal(isAllowedURL("https://github.com.evil.example/release"), false);
});

test("parses and enforces sha256 checksums", () => {
  const body = Buffer.from("rtc-cli");
  const hash = crypto.createHash("sha256").update(body).digest("hex");
  assert.equal(expectedChecksum(`${hash}  convo-cli_1.0.0_linux_amd64.tar.gz\n`, "convo-cli_1.0.0_linux_amd64.tar.gz"), hash);
  assert.doesNotThrow(() => verify(body, hash));
  assert.throws(() => verify(body, "0".repeat(64)), /checksum mismatch/);
});

test("uses BytePlus registry mirrors", () => {
  const registry = process.env.npm_config_registry;
  delete process.env.npm_config_registry;
  try {
    const result = urls("1.2.3", "archive.tgz");
    assert.match(result[0], /registry\.npmmirror\.com/);
    assert.ok(result.every((value) => value.includes("/binary/convo-cli/")));
    assert.ok(result.some((value) => value.includes("registry.npmjs.org")));
  } finally {
    if (registry === undefined) delete process.env.npm_config_registry;
    else process.env.npm_config_registry = registry;
  }
});

test("uses a bundled archive when the package contains one", () => {
  const packageRoot = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-package-"));
  try {
    const artifacts = path.join(packageRoot, "artifacts");
    fs.mkdirSync(artifacts);
    fs.writeFileSync(path.join(artifacts, "convo-cli_test.tar.gz"), "bundled-binary");
    assert.equal(readBundledArchive(packageRoot, "convo-cli_test.tar.gz").toString(), "bundled-binary");
    assert.equal(readBundledArchive(packageRoot, "missing.tar.gz"), null);
  } finally {
    fs.rmSync(packageRoot, { recursive: true, force: true });
  }
});

test("Windows replacement removes stale backup and keeps verified candidate", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-installer-test-"));
  const source = path.join(dir, "new.exe");
  const destination = path.join(dir, "convo-cli.exe");
  fs.writeFileSync(source, "new");
  fs.writeFileSync(destination, "old");
  fs.writeFileSync(`${destination}.old`, "stale");
  replaceBinary(source, destination, "2.0.0", "win32", () => ({ status: 0, stdout: "convo-cli 2.0.0\n" }));
  assert.equal(fs.readFileSync(destination, "utf8"), "new");
  assert.equal(fs.readFileSync(`${destination}.old`, "utf8"), "old");
  fs.rmSync(dir, { recursive: true, force: true });
});

test("Windows replacement restores previous binary when verification fails", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-installer-test-"));
  const source = path.join(dir, "new.exe");
  const destination = path.join(dir, "convo-cli.exe");
  fs.writeFileSync(source, "new");
  fs.writeFileSync(destination, "old");
  assert.throws(() => replaceBinary(source, destination, "2.0.0", "win32", () => ({ status: 1, stderr: "broken" })), /exited 1/);
  assert.equal(fs.readFileSync(destination, "utf8"), "old");
  assert.equal(fs.existsSync(`${destination}.old`), false);
  fs.rmSync(dir, { recursive: true, force: true });
});

test("Windows replacement preserves a sole existing backup", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "convo-cli-installer-test-"));
  const source = path.join(dir, "new.exe");
  const destination = path.join(dir, "convo-cli.exe");
  fs.writeFileSync(source, "new");
  fs.writeFileSync(`${destination}.old`, "recoverable");
  replaceBinary(source, destination, "2.0.0", "win32", () => ({ status: 0, stdout: "convo-cli 2.0.0\n" }));
  assert.equal(fs.readFileSync(destination, "utf8"), "new");
  assert.equal(fs.readFileSync(`${destination}.old`, "utf8"), "recoverable");
  fs.rmSync(dir, { recursive: true, force: true });
});

for (const platform of ["linux", "darwin"]) {
  test(`${platform} replacement preserves the installed binary when verification fails`, () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "installer-test-"));
    try {
      const source = path.join(dir, "new");
      const destination = path.join(dir, "cli");
      fs.writeFileSync(source, "broken");
      fs.writeFileSync(destination, "working");
      fs.chmodSync(destination, 0o755);
      assert.throws(() => replaceBinary(source, destination, "2.0.0", platform,
        () => ({ status: 1, stderr: "broken" })), /exited 1/);
      assert.equal(fs.readFileSync(destination, "utf8"), "working");
      assert.deepEqual(fs.readdirSync(dir).sort(), ["cli", "new"]);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });

  test(`${platform} replacement installs a verified executable`, () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "installer-test-"));
    try {
      const source = path.join(dir, "new");
      const destination = path.join(dir, "cli");
      fs.writeFileSync(source, "working");
      replaceBinary(source, destination, "2.0.0", platform, (candidate) => {
        assert.notEqual(candidate, destination);
        assert.equal(fs.readFileSync(candidate, "utf8"), "working");
        return { status: 0, stdout: "cli 2.0.0\n" };
      });
      assert.equal(fs.readFileSync(destination, "utf8"), "working");
      if (process.platform !== "win32") assert.equal(fs.statSync(destination).mode & 0o777, 0o755);
      assert.deepEqual(fs.readdirSync(dir).sort(), ["cli", "new"]);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
}
