import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import test from "node:test";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

test("updates only the Dispatcharr catalog entry from release checksums", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "dispatcharr-catalog-"));
  const catalogDir = path.join(root, "catalog");
  fs.mkdirSync(catalogDir);
  fs.writeFileSync(path.join(catalogDir, "manifest.json"), JSON.stringify({ plugins: [
    { manifest: { plugin_id: "example.other", version: "1.0.0" } },
    { manifest: { plugin_id: "silo.ramindex.dispatcharr", version: "0.0.1" }, binaries: {} }
  ] }));
  fs.writeFileSync(path.join(catalogDir, "checksums.txt"), "a".repeat(64) + "  other-1.0.0-linux-amd64\n" + "b".repeat(64) + "  dispatcharr-0.0.1-linux-amd64\n");
  const checksumPath = path.join(root, "release-checksums.txt");
  const platforms = ["darwin-arm64", "linux-amd64", "linux-arm64"];
  fs.writeFileSync(checksumPath, platforms.map((platform, index) => `${String(index + 1).repeat(64)}  dispatcharr-9.8.7-${platform}`).join("\n") + "\n");

  const scriptDir = path.dirname(fileURLToPath(import.meta.url));
  const result = spawnSync(process.execPath, [path.join(scriptDir, "update-catalog.mjs"), "--catalog-dir", catalogDir, "--version", "9.8.7", "--checksums", checksumPath], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  const catalog = JSON.parse(fs.readFileSync(path.join(catalogDir, "manifest.json"), "utf8"));
  const dispatcharr = catalog.plugins.find((entry) => entry.manifest.plugin_id === "silo.ramindex.dispatcharr");
  assert.equal(dispatcharr.manifest.version, "9.8.7");
  assert.equal(dispatcharr.manifest.checksum, "2".repeat(64));
  assert.match(dispatcharr.binaries["linux/arm64"].url, /v9\.8\.7\/dispatcharr-9\.8\.7-linux-arm64$/);
  const catalogChecksums = fs.readFileSync(path.join(catalogDir, "checksums.txt"), "utf8");
  assert.match(catalogChecksums, /other-1\.0\.0-linux-amd64/);
  assert.doesNotMatch(catalogChecksums, /dispatcharr-0\.0\.1/);
  assert.match(catalogChecksums, /dispatcharr-9\.8\.7-linux-amd64/);
});
