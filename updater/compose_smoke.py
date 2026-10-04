"""Run INSIDE a disposable Compose updater container. Never use a real installation.
Uses locally built images to exercise orchestration independently of release publication.
"""
import os
import sys
from update import Updater, run

if os.environ.get('TULLIPS_DISPOSABLE_UPDATE_TEST') != 'yes':
    raise SystemExit('This destructive check requires a dedicated disposable Compose project')
u = Updater(sys.argv[1])
if u.env.get('COMPOSE_PROJECT_NAME') != 'tullips-updater-qa':
    raise SystemExit('Refusing to test outside the tullips-updater-qa project')
with u.lock():
    original = u.state()['version']
    images = u.deployment.snapshot()['override']['services']
    target = {'version': '0.1.1', 'minimum_version': '0.1.0', 'rollback_versions': [original],
              'images': {'backend': images['api']['image'], 'web': images['web']['image']}}
    u.save(available=[target])
    # Release authentication and downloads have separate tests. Images are already local.
    u.deployment.prepare = lambda manifest: None
    u.apply('0.1.1')
    assert u.state()['phase'] == 'complete', u.status()
    assert u.sql('SELECT maintenance FROM installation') == 'f'
    print('PASS: complete Compose update, migrations, backup, health checks')
    target = target | {'version': '0.1.2', 'rollback_versions': ['0.1.1']}
    u.save(available=[target])
    real_health = u.deployment.healthy
    count = 0
    def fail_health_once():
        global count
        count += 1
        if count == 1: raise RuntimeError('simulated unhealthy candidate')
        real_health()
    u.deployment.healthy = fail_health_once
    u.apply('0.1.2')
    assert u.state()['phase'] == 'rolled_back', u.status()
    assert u.state()['version'] == '0.1.1'
    print('PASS: failed health check restores previous Compose image IDs')
    u.deployment.healthy = real_health
    target = target | {'version': '0.1.3', 'rollback_versions': []}
    u.save(available=[target])
    def failed_migration(manifest):
        u.sql('CREATE TABLE failed_update_marker(id int)')
        raise RuntimeError('simulated incompatible migration failure')
    u.deployment.migrate = failed_migration
    u.apply('0.1.3')
    assert u.state()['phase'] == 'awaiting_restore', u.status()
    assert u.sql('SELECT maintenance FROM installation') == 't'
    try:
        u.restore()
        raise AssertionError('restoration was permitted without approval')
    except ValueError: pass
    u.restore(confirm='0.1.1')
    assert u.state()['phase'] == 'rolled_back', u.status()
    assert u.sql("SELECT to_regclass('failed_update_marker') IS NULL") == 't'
    assert u.sql('SELECT maintenance FROM installation') == 'f'
    print('PASS: incompatible migration waits for approval, then restores database and services')
