#!/usr/bin/env bash
# Copyright (c) 2026 BytePlus Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
preflight="$repo_root/scripts/preflight-public-release.sh"

[[ -x "$preflight" ]] || {
  echo "preflight-public-release-test: missing executable $preflight" >&2
  exit 1
}

fixture="$(mktemp -d)"
trap 'rm -rf "$fixture"' EXIT
git -C "$repo_root" archive HEAD | tar -x -C "$fixture"
git -C "$fixture" init -q
git -C "$fixture" config user.name Test
git -C "$fixture" config user.email test@example.com
source_version=9.8.6
(cd "$fixture" && go run ./internal/releasecmd source-version --repo "$fixture" --version "$source_version" >/dev/null)
printf '\n## %s\n\n- Release fixture.\n' "$source_version" >> "$fixture/CHANGELOG.md"
git -C "$fixture" add .
git -C "$fixture" commit -qm fixture
repo_root="$fixture"
preflight="$repo_root/scripts/preflight-public-release.sh"

source_stability=stable
[[ "$source_version" != *-* ]] || source_stability=prerelease
source_output="$($preflight --stability "$source_stability" --version "$source_version" --ref HEAD)"
grep -Fqx "preflight-public-release: version=$source_version" <<< "$source_output"
grep -Fqx "preflight-public-release: source_commit=$(git -C "$repo_root" rev-parse HEAD)" <<< "$source_output"
grep -Eq '^preflight-public-release: prepared_skills=[1-9][0-9]*$' <<< "$source_output"
grep -Fqx "preflight-public-release: contract=passed" <<< "$source_output"

if "$preflight" --stability stable --version 9.8.7 --ref HEAD >/dev/null 2>&1; then
  echo "preflight-public-release-test: mismatched committed version passed" >&2
  exit 1
fi

if "$preflight" --stability stable --version 1.2 --ref HEAD >/dev/null 2>&1; then
  echo "preflight-public-release-test: invalid stable version passed" >&2
  exit 1
fi

for reserved_version in 0.0.0-dev 0.0.0-snapshot; do
  if "$preflight" --stability prerelease --version "$reserved_version" --ref HEAD >/dev/null 2>&1; then
    echo "preflight-public-release-test: reserved version passed: $reserved_version" >&2
    exit 1
  fi
done

echo "preflight-public-release-test: passed"
