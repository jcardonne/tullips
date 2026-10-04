"""Opt-in destructive checks against a DISPOSABLE database only."""
import os
from pathlib import Path
import tempfile
import unittest
from update import Updater, atomic
from test_update import FakeDeployment, MANIFEST


@unittest.skipUnless(os.getenv('UPDATER_TEST_DATABASE_URL'), 'set UPDATER_TEST_DATABASE_URL to a disposable database')
class DatabaseRecoveryTest(unittest.TestCase):
    def test_backup_restore_and_admin(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            (root/'env').write_text('UPDATER_TOKEN='+'q'*40+'\nDATABASE_URL='+os.environ['UPDATER_TEST_DATABASE_URL']+'\nENCRYPTION_KEY=test-preserved-key\n')
            atomic(root/'config.json', {'mode':'compose','repository':'jcardonne/tullips','version':'1.0.0','env_file':str(root/'env'),'state_dir':str(root/'state')})
            u=Updater(root/'config.json')
            u.sql('CREATE TABLE IF NOT EXISTS installation(id boolean PRIMARY KEY,admin_user_id text,bootstrap_open boolean,maintenance boolean); INSERT INTO installation VALUES(true,NULL,false,false) ON CONFLICT DO NOTHING; CREATE TABLE IF NOT EXISTS "user"(id text PRIMARY KEY,email text); INSERT INTO "user" VALUES(\'one\',\'admin@example.test\') ON CONFLICT DO NOTHING; CREATE TABLE updater_sentinel(value text); INSERT INTO updater_sentinel VALUES(\'before-update\');')
            u.admin('admin@example.test')
            self.assertEqual(u.sql('SELECT admin_user_id FROM installation'), 'one')
            with self.assertRaises(ValueError): u.admin("missing' OR true; --")
            u.save(previous={'images':'before'},previous_version='1.0.0',target=MANIFEST,maintenance_entered=True,migration_started=True)
            u.maintenance(True)
            u.backup()
            self.assertEqual((Path(u.state()['backup'])/'environment').read_text(), (root/'env').read_text())
            u.sql("UPDATE updater_sentinel SET value='after-update'; CREATE TABLE failed_release_table(id int);")
            u.save(phase='awaiting_restore')
            u.deployment=FakeDeployment()
            u.restore(confirm='1.0.0')
            self.assertEqual(u.state()['phase'], 'rolled_back')
            self.assertEqual(u.sql('SELECT value FROM updater_sentinel'), 'before-update')
            self.assertEqual(u.sql("SELECT to_regclass('failed_release_table') IS NULL"), 't')
            self.assertEqual(u.sql('SELECT maintenance FROM installation'), 'f')
            u.sql('DROP TABLE updater_sentinel;')
