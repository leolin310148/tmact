#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/tmact-install-skills-test.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT

TEST_HOME="$TEST_ROOT/home"
SKILLS=(tmact-loop tmact-dispatch agent-loop handoff)
DESTINATIONS=("$TEST_HOME/.codex/skills" "$TEST_HOME/.claude/skills")

for destination_root in "${DESTINATIONS[@]}"; do
  mkdir -p "$destination_root"
  for skill in "${SKILLS[@]}"; do
    ln -s "$REPO_ROOT/skills/$skill" "$destination_root/$skill"
  done
done

duplicate="$TEST_HOME/.codex/skills/tmact-dispatch.backup-20260722-120000"
orphan="$TEST_HOME/.codex/skills/retired-skill.backup-20260722-120000"
inactive="$TEST_HOME/.codex/skills/handoff.backup-empty"
ln -s "$REPO_ROOT/skills/tmact-dispatch" "$duplicate"
mkdir -p "$orphan" "$inactive"
printf '%s\n' '---' 'name: retired-skill' 'description: synthetic test fixture' '---' >"$orphan/SKILL.md"

output="$TEST_ROOT/check-output"
HOME="$TEST_HOME" "$REPO_ROOT/scripts/install-skills.sh" --check >"$output" 2>&1

assert_contains() {
  local expected="$1"
  if ! grep -Fq "$expected" "$output"; then
    echo "missing expected output: $expected" >&2
    cat "$output" >&2
    exit 1
  fi
}

assert_contains "warning: active duplicate backup skill: $duplicate (duplicates $TEST_HOME/.codex/skills/tmact-dispatch)"
assert_contains "warning: active orphan backup skill: $orphan (no managed canonical skill named retired-skill)"

if grep -Fq "$inactive" "$output"; then
  echo "inactive backup without SKILL.md was reported" >&2
  cat "$output" >&2
  exit 1
fi

for path in "$duplicate" "$orphan" "$inactive"; do
  if [[ ! -e "$path" ]]; then
    echo "read-only check removed $path" >&2
    exit 1
  fi
done

echo "install-skills checks passed"
