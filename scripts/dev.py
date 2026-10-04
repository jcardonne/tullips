#!/usr/bin/env python3
"""Run a native development process with local .env and loopback PostgreSQL."""
import os
from pathlib import Path
import sys
import subprocess
root = Path(__file__).resolve().parent.parent
config = dict(line.split('=', 1) for line in (root / '.env').read_text().splitlines() if line and not line.startswith('#') and '=' in line)
env = os.environ | config
env['DATABASE_URL'] = f"postgres://tullips:{config['POSTGRES_PASSWORD']}@127.0.0.1:54329/tullips?sslmode=disable"
env['BETTER_AUTH_URL'] = config['APP_URL']
env['API_URL'] = 'http://127.0.0.1:8080'
commands = {'api': ('backend', ['go', 'run', './cmd/api']), 'worker': ('backend', ['go', 'run', './cmd/worker']), 'web': ('web', ['npm', 'run', 'dev']), 'migrate': ('web', ['npm', 'run', 'auth:migrate'])}
if len(sys.argv) != 2 or sys.argv[1] not in commands:
    raise SystemExit('Usage: python3 scripts/dev.py api|worker|web|migrate')
if sys.argv[1] == "migrate":
    subprocess.run(["go", "run", "./cmd/migrate"], cwd=root / "backend", env=env, check=True)
folder, command = commands[sys.argv[1]]
os.chdir(root / folder)
os.execvpe(command[0], command, env)
