import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('deploy_skills', Path(__file__).with_name('deploy-skills.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class DeploySkillsTest(unittest.TestCase):
    def test_shared_symlink_update_verify_and_rollback(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            stage = home / 'stage'
            source = stage / 'source' / 'tmact-ask'
            source.mkdir(parents=True)
            (source / 'SKILL.md').write_text('new')
            vendor = home / 'vendor' / 'tmact-ask'
            vendor.mkdir(parents=True)
            (vendor / 'SKILL.md').write_text('old')
            (vendor / 'obsolete.txt').write_text('old helper')
            for agent in ('.codex', '.claude'):
                link = home / agent / 'skills' / 'tmact-ask'
                link.parent.mkdir(parents=True)
                link.symlink_to(vendor)
            with patch.object(Path, 'home', return_value=home):
                module.run('apply', stage)
                module.run('verify', stage)
                self.assertFalse((vendor / 'obsolete.txt').exists())
                self.assertTrue((home / '.codex/skills/tmact-ask').is_symlink())
                (vendor / 'SKILL.md').write_text('tampered')
                with self.assertRaisesRegex(RuntimeError, 'mismatch'):
                    module.run('verify', stage)
                module.run('rollback', stage)
                self.assertEqual((vendor / 'SKILL.md').read_text(), 'old')
                self.assertTrue((vendor / 'obsolete.txt').exists())
                module.run('rollback', stage)

    def test_missing_destination_does_not_modify_other_skills(self):
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            source = home / 'stage/source/tmact-ask'
            source.mkdir(parents=True)
            (source / 'SKILL.md').write_text('new')
            old = home / '.codex/skills/tmact-ask'
            old.mkdir(parents=True)
            (old / 'SKILL.md').write_text('old')
            with patch.object(Path, 'home', return_value=home):
                with self.assertRaises(FileNotFoundError):
                    module.run('apply', home / 'stage')
            self.assertEqual((old / 'SKILL.md').read_text(), 'old')


if __name__ == '__main__':
    unittest.main()
