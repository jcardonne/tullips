#!/usr/bin/env python3
"""Set up Compose, or migrate its existing configuration without changing secrets."""
from pathlib import Path
import argparse
import os
import secrets
import sys
import subprocess
import time
import shutil

root = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(root / 'updater'))
from update import atomic, env_file
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--migrate', action='store_true', help='add the updater to an existing installation')
args = parser.parse_args()
os.umask(0o077)
target = root / '.env'
if target.exists() and not args.migrate:
    raise SystemExit('.env already exists. Use --migrate to add the updater while preserving your configuration.')
if not target.exists():
    text = (root / '.env.example').read_text()
    for name in ('POSTGRES_PASSWORD', 'BETTER_AUTH_SECRET', 'INTERNAL_API_SECRET', 'ENCRYPTION_KEY', 'LITELLM_MASTER_KEY'):
        lines = text.splitlines()
        text = '\n'.join(f'{name}={secrets.token_urlsafe(36)}' if line.startswith(name + '=') else line for line in lines) + '\n'
    target.write_text(text)
if args.migrate:
    backup = root / '.tullips-updater' / ('adoption-backup-' + str(time.time_ns()))
    backup.mkdir(parents=True, mode=0o700)
    compose = ['docker', 'compose', '--project-directory', str(root), '--env-file', str(target), '-f', str(root / 'compose.yaml')]
    subprocess.run([*compose, 'stop', '--timeout', '330', 'web', 'api', 'worker'], check=True)
    try:
        with (backup / 'database.dump').open('wb') as output:
            subprocess.run([*compose, 'exec', '-T', 'postgres', 'pg_dump', '-U', 'tullips', '-d', 'tullips', '-Fc'], stdout=output, check=True)
        with (backup / 'database.dump').open('rb') as source:
            subprocess.run([*compose, 'exec', '-T', 'postgres', 'pg_restore', '--list'], stdin=source, stdout=subprocess.DEVNULL, check=True)
        shutil.copy2(target, backup / 'environment')
    except Exception:
        subprocess.run([*compose, 'start', 'api', 'worker', 'web'], check=True)
        raise
    print('Existing application stopped. Adoption backup saved at', backup)
config = env_file(target)
additions = {
    'UPDATER_TOKEN': secrets.token_urlsafe(36),
    'TULLIPS_PROJECT_DIR': str(root),
    'COMPOSE_FILE': 'compose.yaml:.tullips-updater/release.compose.json',
    'DATABASE_URL': f"postgres://tullips:{config['POSTGRES_PASSWORD']}@postgres:5432/tullips?sslmode=disable",
}
with target.open('a') as output:
    for key, value in additions.items():
        if not config.get(key): output.write(f'\n{key}={value}\n')
target.chmod(0o600)
state = root / '.tullips-updater'
state.mkdir(exist_ok=True, mode=0o700)
if not (state / 'release.compose.json').exists(): atomic(state / 'release.compose.json', {'services': {}})
if not (state / 'updater.json').exists():
    atomic(state / 'updater.json', {'mode': 'compose', 'repository': 'jcardonne/tullips',
        'project_dir': str(root), 'state_dir': str(state), 'env_file': str(target),
        'version': (root / 'VERSION').read_text().strip(), 'listen': '0.0.0.0:8090',
        'api_url': 'http://api:8080/healthz', 'web_url': 'http://web:3000/healthz',
        'worker_url': 'http://worker:8081/healthz', 'retention': 3})
print('Configuration ready. Start with: docker compose up -d --build')
if args.migrate: print('Assign an existing user: docker compose exec updater python3 /opt/updater/update.py --config ' + str(state / 'updater.json') + ' admin EMAIL')
