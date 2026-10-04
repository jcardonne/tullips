import contextlib
import copy
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from update import Updater, atomic, version, validate_manifest
from adapters import Deployment

REPO = 'jcardonne/tullips'
MANIFEST = {'format': 1, 'updater_protocol': 1, 'repository': REPO, 'version': '1.1.0', 'minimum_version': '1.0.0', 'rollback_versions': [], 'images': {'backend': 'ghcr.io/jcardonne/tullips-backend@sha256:' + 'a'*64, 'web': 'ghcr.io/jcardonne/tullips-web@sha256:' + 'b'*64}, 'source_sha256': 'c'*64}


class FakeDeployment:
    def __init__(self, fail=None): self.calls, self.fail = [], fail
    def call(self, name):
        self.calls.append(name)
        if self.fail == name:
            self.fail = None
            raise RuntimeError('simulated ' + name + ' failure')
    def preflight(self): self.call('preflight')
    def snapshot(self): self.call('snapshot'); return {'image': 'old'}
    def prepare(self, m): self.call('prepare')
    def stop(self): self.call('stop')
    def migrate(self, m): self.call('migrate')
    def deploy(self, m): self.call('deploy')
    def healthy(self): self.call('healthy')
    def restore(self, previous): self.call('restore')


class UpdaterTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        environment = self.root/'environment'
        environment.write_text('UPDATER_TOKEN='+'x'*40+'\nDATABASE_URL=postgres://localhost/test\n')
        atomic(self.root/'config.json', {'mode':'compose', 'repository':REPO, 'state_dir':str(self.root/'state'), 'env_file':str(environment), 'version':'1.0.0'})
        self.u = Updater(self.root/'config.json')
        self.u.deployment = FakeDeployment()
        self.u.save(available=[copy.deepcopy(MANIFEST)])
        self.maintenance = []
        self.u.maintenance = self.maintenance.append
        def backup():
            self.u.deployment.call('backup')
            self.u.save(backup_complete=True)
        self.u.backup = backup
        self.u.prune = lambda: None

    def test_success_and_order(self):
        self.u.apply('1.1.0')
        self.assertEqual(self.u.state()['phase'], 'complete')
        self.assertEqual(self.u.state()['version'], '1.1.0')
        self.assertEqual(self.maintenance, [True, False])
        self.assertEqual(self.u.deployment.calls, ['preflight','snapshot','prepare','stop','backup','migrate','deploy','healthy'])

    def test_backup_failure_never_migrates_and_restarts_previous(self):
        self.u.deployment.fail = 'backup'
        self.u.apply('1.1.0')
        self.assertNotIn('migrate', self.u.deployment.calls)
        self.assertEqual(self.u.state()['phase'], 'rolled_back')
        self.assertEqual(self.u.state()['version'], '1.0.0')

    def test_incompatible_migration_requires_explicit_restore(self):
        self.u.deployment.fail = 'migrate'
        self.u.apply('1.1.0')
        self.assertEqual(self.u.state()['phase'], 'awaiting_restore')
        self.assertNotIn('restore', self.u.deployment.calls)
        self.assertNotIn(False, self.maintenance)
        with self.assertRaises(ValueError): self.u.restore(confirm='1.1.0')
        with self.assertRaises(ValueError): self.u.apply('1.1.0')

    def test_safe_health_failure_rolls_back(self):
        m = copy.deepcopy(MANIFEST); m['rollback_versions']=['1.0.0']
        self.u.save(available=[m])
        self.u.deployment.fail='healthy'
        self.u.apply('1.1.0')
        self.assertEqual(self.u.state()['phase'], 'rolled_back')
        self.assertEqual(self.u.state()['failed_version'], '1.1.0')
        with self.assertRaises(ValueError): self.u.apply('1.1.0', automatic=True)

    def test_interrupted_restore_never_resumes_normal_traffic(self):
        self.u.save(phase='restoring', migration_started=True, restore_started=True, maintenance_entered=True,
                    previous_version='1.0.0', target=MANIFEST | {'rollback_versions':['1.0.0']})
        self.u.recover()
        self.assertEqual(self.u.state()['phase'], 'awaiting_restore')
        self.assertNotIn(False, self.maintenance)

    def test_interrupt_before_migration_safely_recovers(self):
        self.u.save(phase='backing_up', maintenance_entered=True, previous_version='1.0.0', previous={'image':'old'})
        self.u.recover()
        self.assertEqual(self.u.state()['phase'], 'rolled_back')

    def test_failed_preflight_does_not_stop_application(self):
        self.u.deployment.fail='preflight'
        self.u.apply('1.1.0')
        self.assertEqual(self.u.state()['phase'], 'failed')
        self.assertEqual(self.maintenance, [])

    def test_major_updates_downgrades_and_lock(self):
        with self.assertRaises(ValueError): self.u.apply('2.0.0', automatic=True)
        with self.assertRaises(ValueError): self.u.apply('0.9.0')
        with self.u.lock():
            with self.assertRaises(RuntimeError):
                with self.u.lock(): pass

    def test_manifest_and_versions_reject_untrusted_inputs(self):
        self.assertEqual(version('1.10.0'), (1,10,0))
        for bad in ('v1.0.0','1.0','1.0.0-rc.1','01.0.0','1.0.0;id',None):
            with self.assertRaises(ValueError): version(bad)
        validate_manifest(MANIFEST, REPO)
        for m in (MANIFEST | {'repository':'other/repo'}, MANIFEST | {'updater_protocol':2}, MANIFEST | {'images': {'backend':'evil/image:latest','web':'evil'}}):
            with self.assertRaises(ValueError): validate_manifest(m, REPO)

    def test_source_changes_are_preserved(self):
        from update import sha
        source=self.root/'source'; source.mkdir()
        (source/'file').write_text('original')
        atomic(source/'source-files.json', {'file':sha(source/'file')})
        deployment=Deployment(self.u)
        deployment.check_source(source)
        (source/'file').write_text('local changes')
        with self.assertRaises(ValueError): deployment.check_source(source)
        self.assertEqual((source/'file').read_text(), 'local changes')

    def test_kubernetes_jobs_and_coolify_use_pinned_images(self):
        from types import SimpleNamespace
        from adapters import Deployment
        self.u.c |= {'mode':'coolify', 'applications':{'api':'a','web':'b','worker':'c'}}
        d=Deployment(self.u); calls=[]
        d.coolify=lambda *args: calls.append(args)
        d.coolify_deploy=lambda uuid: None
        d.deploy(MANIFEST)
        self.assertEqual(calls[0][2]['docker_registry_image_tag'], 'sha256-'+'a'*64)
        self.assertEqual(calls[0][2]['docker_registry_image_name'], 'ghcr.io/jcardonne/tullips-backend')

    def test_connection_url_is_not_passed_as_a_literal_database_name(self):
        from update import database_env
        result = database_env({'DATABASE_URL':'postgresql://u:p%40ss@db.example:5433/app?sslmode=require'})
        self.assertEqual(result['PGDATABASE'], 'app')
        self.assertEqual(result['PGHOST'], 'db.example')
        self.assertEqual(result['PGPASSWORD'], 'p@ss')
        self.assertEqual(result['PGSSLMODE'], 'require')
        with self.assertRaises(ValueError): database_env({'DATABASE_URL':'https://evil.example'})

    def test_kubernetes_installer_generates_scoped_updater_and_stopped_apps(self):
        import sys
        sys.path.insert(0, str(Path(__file__).resolve().parent.parent / 'scripts'))
        from install import kube_resources
        resources = kube_resources({'namespace':'test','mode':'kubernetes'}, MANIFEST | {'images': MANIFEST['images'] | {'updater':'example@sha256:'+'d'*64}}, {'DATABASE_URL':'postgres://test/test'}, '20Gi')
        deployments = [r for r in resources if r['kind']=='Deployment']
        self.assertEqual(len(deployments), 4)
        self.assertTrue(all(r['spec']['replicas']==0 for r in deployments))
        self.assertTrue(all(r['metadata']['namespace']=='test' for r in resources))
        api = next(r for r in deployments if r['metadata']['name']=='tullips-api')
        self.assertFalse(api['spec']['template']['spec']['automountServiceAccountToken'])
        role = next(r for r in resources if r['kind']=='Role')
        self.assertIn('watch', role['rules'][0]['verbs'])
        self.assertEqual(role['rules'][0]['resourceNames'], ['tullips-api','tullips-web','tullips-worker'])

    def test_settings_strict_boolean_and_status_no_secrets(self):
        with self.assertRaises(ValueError): self.u.settings('true')
        self.u.settings(True)
        self.assertTrue(self.u.status()['automatic'])
        self.u.save(previous={'password':'secret'}, target={'password':'secret'})
        self.assertNotIn('secret', json.dumps(self.u.status()))


if __name__ == '__main__': unittest.main()
