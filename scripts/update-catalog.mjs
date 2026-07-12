#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const options = parseOptions(process.argv.slice(2));
const catalogDir = requiredOption(options, "catalog-dir");
const version = requiredOption(options, "version").replace(/^v/, "");
const checksumsPath = requiredOption(options, "checksums");
const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const pluginManifest = readJSON(path.join(repoRoot, "manifest.json"));
const releaseBase = `https://github.com/theramindex/silo-plugin-dispatcharr/releases/download/v${version}`;
const platforms = [
  { key: "darwin/arm64", os: "darwin", arch: "arm64" },
  { key: "linux/amd64", os: "linux", arch: "amd64" },
  { key: "linux/arm64", os: "linux", arch: "arm64" }
];
const releaseChecksums = parseChecksums(fs.readFileSync(checksumsPath, "utf8"));
const binaries = {};

for (const platform of platforms) {
  const filename = `dispatcharr-${version}-${platform.os}-${platform.arch}`;
  const checksum = releaseChecksums.get(filename);
  if (!checksum) fail(`missing checksum for ${filename}`);
  binaries[platform.key] = { url: `${releaseBase}/${filename}`, checksum };
}

const catalogManifestPath = path.join(catalogDir, "manifest.json");
const catalog = readJSON(catalogManifestPath);
const entry = (catalog.plugins || []).find((candidate) => candidate?.manifest?.plugin_id === pluginManifest.plugin_id);
if (!entry) fail(`catalog does not contain ${pluginManifest.plugin_id}`);

entry.manifest.version = version;
entry.manifest.checksum = binaries["linux/amd64"].checksum;
entry.manifest.silo_api_version = pluginManifest.silo_api_version;
entry.manifest.supported_platforms = pluginManifest.supported_platforms;
entry.manifest.capabilities = pluginManifest.capabilities;
entry.manifest.category = pluginManifest.category;
entry.repo_url = "https://github.com/theramindex/silo-plugin-dispatcharr";
entry.checksums_url = `${releaseBase}/checksums.txt`;
entry.binaries = binaries;
fs.writeFileSync(catalogManifestPath, `${JSON.stringify(catalog, null, 2)}\n`);

const catalogChecksumsPath = path.join(catalogDir, "checksums.txt");
const retained = fs.readFileSync(catalogChecksumsPath, "utf8").split(/\r?\n/).filter(Boolean).filter((line) => !/\sdispatcharr-[^\s]+$/.test(line));
for (const platform of platforms) {
  const filename = `dispatcharr-${version}-${platform.os}-${platform.arch}`;
  retained.push(`${releaseChecksums.get(filename)}  ${filename}`);
}
retained.sort((left, right) => left.split(/\s+/).at(-1).localeCompare(right.split(/\s+/).at(-1)));
fs.writeFileSync(catalogChecksumsPath, `${retained.join("\n")}\n`);

function parseOptions(args) {
  const result = {};
  for (let index = 0; index < args.length; index += 2) {
    const key = String(args[index] || "").replace(/^--/, "");
    result[key] = args[index + 1];
  }
  return result;
}

function requiredOption(values, key) {
  const value = String(values[key] || "").trim();
  if (!value) fail(`--${key} is required`);
  return value;
}

function readJSON(filename) {
  return JSON.parse(fs.readFileSync(filename, "utf8"));
}

function parseChecksums(contents) {
  const result = new Map();
  for (const line of contents.split(/\r?\n/)) {
    const match = line.trim().match(/^([a-f0-9]{64})\s+\*?(.+)$/i);
    if (match) result.set(match[2], match[1].toLowerCase());
  }
  return result;
}

function fail(message) {
  process.stderr.write(`${message}\n`);
  process.exit(1);
}
