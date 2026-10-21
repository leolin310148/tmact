#!/usr/bin/env python3
"""Stage, verify and reversibly replace canonical skills, preserving agent links."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import sys


def digest(root):
    result = {}
    for item in sorted(root.rglob('*')):
        if item.is_symlink():
            raise RuntimeError('skill payload contains symlink: ' + str(item))
        if item.is_file():
            result[str(item.relative_to(root))] = hashlib.sha256(item.read_bytes()).hexdigest()
    return result


def targets(source):
    found = {}
    for agent in ('.codex', '.claude'):
        for skill in sorted(source.iterdir()):
            if not (skill / 'SKILL.md').is_file():
                raise RuntimeError('invalid canonical skill: ' + skill.name)
            link = Path.home() / agent / 'skills' / skill.name
            target = link.resolve(strict=True)
            if not target.is_dir() or not target.is_relative_to(Path.home().resolve()):
                raise RuntimeError('invalid skill destination: ' + str(link))
            if target in found and found[target] != skill:
                raise RuntimeError('conflicting skill destinations')
            found[target] = skill
    return found


def run(mode, stage):
    source = stage / 'source'
    manifest = stage / 'rollback.json'
    if mode == 'preflight':
        targets(source)
        for skill in source.iterdir():
            digest(skill)
        print('skill destinations ready')
    elif mode == 'verify':
        for target, skill in targets(source).items():
            if digest(target) != digest(skill):
                raise RuntimeError('skill content mismatch: ' + str(target))
        print('skills verified: all canonical files, Claude + Codex')
    elif mode == 'apply':
        if manifest.exists():
            raise RuntimeError('stage already applied')
        entries = []
        # Resolve and validate every destination before modifying any of them.
        destinations = targets(source)
        for number, (target, skill) in enumerate(destinations.items()):
            backup = stage / ('backup-' + str(number))
            prepared = stage / ('prepared-' + str(number))
            shutil.copytree(skill, prepared)
            entries.append([str(target), str(backup)])
            manifest.write_text(json.dumps(entries))
            target.rename(backup)
            prepared.rename(target)
        run('verify', stage)
    elif mode == 'rollback':
        if manifest.exists():
            for target_name, backup_name in reversed(json.loads(manifest.read_text())):
                target, backup = Path(target_name), Path(backup_name)
                if backup.exists():
                    if target.exists():
                        shutil.rmtree(target)
                    backup.rename(target)
            manifest.unlink()
        print('skills rollback complete')
    else:
        raise RuntimeError('unknown mode')


if __name__ == '__main__':
    run(sys.argv[1], Path(sys.argv[2]).resolve())
