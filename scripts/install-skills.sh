#!/usr/bin/env bash

set -euo pipefail

MODE="plan"
BACKUP_EXISTING=0

usage() {
  cat <<'EOF'
usage: scripts/install-skills.sh [--check | --execute] [--backup-existing]

Link tmact-owned skills from this checkout into ~/.codex/skills and
~/.claude/skills. The default is a read-only plan.

  --check            verify every expected link; make no changes
  --execute          create missing links
  --backup-existing  with --execute, move conflicting paths to timestamped backups
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check)
      [[ "$MODE" == "plan" ]] || { echo "choose only one of --check or --execute" >&2; exit 2; }
      MODE="check"
      ;;
    --execute)
      [[ "$MODE" == "plan" ]] || { echo "choose only one of --check or --execute" >&2; exit 2; }
      MODE="execute"
      ;;
    --backup-existing)
      BACKUP_EXISTING=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
done

if [[ "$BACKUP_EXISTING" -eq 1 && "$MODE" != "execute" ]]; then
  echo "--backup-existing requires --execute" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SKILLS=(tmact-loop tmact-dispatch agent-loop handoff)
DESTINATIONS=("$HOME/.codex/skills" "$HOME/.claude/skills")
STAMP="$(date +%Y%m%d-%H%M%S)"
FAILED=0

for skill in "${SKILLS[@]}"; do
  source_path="$REPO_ROOT/skills/$skill"
  if [[ ! -f "$source_path/SKILL.md" ]]; then
    echo "missing canonical skill: $source_path/SKILL.md" >&2
    exit 1
  fi

  for destination_root in "${DESTINATIONS[@]}"; do
    destination="$destination_root/$skill"
    current=""
    if [[ -L "$destination" ]]; then
      current="$(readlink "$destination")"
    fi

    if [[ "$current" == "$source_path" ]]; then
      echo "ok: $destination -> $source_path"
      continue
    fi

    if [[ "$MODE" == "check" ]]; then
      echo "mismatch: $destination (expected -> $source_path)" >&2
      FAILED=1
      continue
    fi

    if [[ "$MODE" == "plan" ]]; then
      if [[ -e "$destination" || -L "$destination" ]]; then
        echo "would replace: $destination -> $source_path (requires --execute --backup-existing)"
      else
        echo "would link: $destination -> $source_path"
      fi
      continue
    fi

    mkdir -p "$destination_root"
    if [[ -e "$destination" || -L "$destination" ]]; then
      if [[ "$BACKUP_EXISTING" -ne 1 ]]; then
        echo "refusing to replace $destination without --backup-existing" >&2
        FAILED=1
        continue
      fi
      backup="$destination.backup-$STAMP"
      if [[ -e "$backup" || -L "$backup" ]]; then
        backup="$backup-$$"
      fi
      mv "$destination" "$backup"
      echo "backup: $destination -> $backup"
    fi
    ln -s "$source_path" "$destination"
    echo "linked: $destination -> $source_path"
  done
done

exit "$FAILED"
