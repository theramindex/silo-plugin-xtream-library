#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

expected_version="${1:-}"
mode="${2:-}"
manifest_version="$(node -p "JSON.parse(require('fs').readFileSync('manifest.json', 'utf8')).version")"

if [[ -n "${expected_version}" && "${manifest_version}" != "${expected_version}" ]]; then
  printf 'manifest version %s does not match release version %s\n' "${manifest_version}" "${expected_version}" >&2
  exit 1
fi

if [[ "${mode}" == "--version-only" ]]; then
  exit 0
fi

go vet ./...
go test ./...
go test -race ./...
node --check internal/plugin/ui/lineup.js
node --check internal/plugin/ui/app.js
node --test scripts/update-catalog.test.mjs
go run . manifest >/dev/null
