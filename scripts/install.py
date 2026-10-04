#!/usr/bin/env python3
"""Install a signed release on Kubernetes, Linux/systemd, or a Coolify server."""
import argparse
import base64
import json
import os
from pathlib import Path
import secrets
import shutil
import sys

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / 'updater'))
from update import Updater, atomic, env_file, run, download, read, validate_manifest, version


def signed_release(repo, v, folder):
    version(v)
    folder.mkdir(parents=True, exist_ok=True)
    base = f'https://github.com/{repo}/releases/download/v{v}'
    download(base + '/release.json', folder / 'release.json', 1024**2)
    download(base + '/release.sigstore.json', folder / 'release.sigstore.json', 8*1024**2)
    run(['gh', 'attestation', 'verify', folder / 'release.json', '--bundle', folder / 'release.sigstore.json',
         '--repo', repo, '--signer-workflow', repo + '/.github/workflows/release.yml', '--source-ref', 'refs/tags/v' + v, '--deny-self-hosted-runners'])
    m = validate_manifest(read(folder / 'release.json'), repo)
    if m['version'] != v: raise ValueError('Release version mismatch')
    m['download_base'] = base
    return m


def kube_resources(c, m, environment, storage):
    namespace = c['namespace']
    def resource(kind, name, **fields):
        return {'apiVersion': 'v1', 'kind': kind, 'metadata': {'name': name, 'namespace': namespace}, **fields}
    runtime = c | {'env_file': '/etc/tullips/environment', 'state_dir': '/var/lib/tullips-updater', 'listen': '0.0.0.0:8090'}
    runtime.update({name + '_url': f'http://tullips-{name}:{port}/healthz' for name, port in (('api', 8080), ('web', 3000), ('worker', 8081))})
    environment |= {'UPDATER_URL': 'http://tullips-updater:8090', 'API_URL': 'http://tullips-api:8080', 'HOST': '0.0.0.0', 'PORT': '3000'}
    resources = [resource('Secret', 'tullips-env', type='Opaque', stringData=environment),
                 resource('Secret', 'tullips-updater-env', type='Opaque', stringData={'environment': '\n'.join(f'{k}={v}' for k, v in environment.items()) + '\n'}),
                 resource('ConfigMap', 'tullips-updater-config', data={'updater.json': json.dumps(runtime)}),
                 resource('PersistentVolumeClaim', 'tullips-backups', spec={'accessModes': ['ReadWriteOnce'], 'resources': {'requests': {'storage': storage}}}),
                 resource('ServiceAccount', 'tullips-updater')]
    resources += [{'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'Role', 'metadata': {'name': 'tullips-updater', 'namespace': namespace}, 'rules': [
        {'apiGroups': ['apps'], 'resources': ['deployments', 'deployments/scale'], 'resourceNames': ['tullips-' + c for c in ('api','web','worker')], 'verbs': ['get', 'list', 'watch', 'patch', 'update']},
        {'apiGroups': ['batch'], 'resources': ['jobs'], 'verbs': ['get', 'list', 'watch', 'create', 'delete']},
        {'apiGroups': [''], 'resources': ['pods'], 'verbs': ['get', 'list', 'watch']},
    ]}, {'apiVersion': 'rbac.authorization.k8s.io/v1', 'kind': 'RoleBinding', 'metadata': {'name': 'tullips-updater', 'namespace': namespace},
        'subjects': [{'kind': 'ServiceAccount', 'name': 'tullips-updater', 'namespace': namespace}],
        'roleRef': {'apiGroup': 'rbac.authorization.k8s.io', 'kind': 'Role', 'name': 'tullips-updater'}}]
    for name, port in (('api', 8080), ('web', 3000), ('worker', 8081), ('updater', 8090)):
        labels = {'app': 'tullips-' + name}
        if name != 'updater': labels['tullips.io/application'] = 'true'
        container = {'name': name, 'image': m['images'][name if name in ('web', 'updater') else 'backend'], 'ports': [{'containerPort': port}], 'envFrom': [{'secretRef': {'name': 'tullips-env'}}]}
        spec = {'terminationGracePeriodSeconds': 330, 'containers': [container]}
        if name == 'worker': container['command'] = ['/app/worker']
        if name != 'updater':
            container['readinessProbe'] = {'httpGet': {'path': '/healthz', 'port': port}, 'periodSeconds': 5}
            spec['automountServiceAccountToken'] = False
        else:
            container['args'] = ['--config', '/etc/tullips/updater.json', 'serve']
            container['volumeMounts'] = [{'name': 'state', 'mountPath': '/var/lib/tullips-updater'}, {'name': 'config', 'mountPath': '/etc/tullips/updater.json', 'subPath': 'updater.json'}, {'name': 'environment', 'mountPath': '/etc/tullips/environment', 'subPath': 'environment'}]
            spec['serviceAccountName'] = 'tullips-updater'
            spec['volumes'] = [{'name': 'state', 'persistentVolumeClaim': {'claimName': 'tullips-backups'}}, {'name': 'config', 'configMap': {'name': 'tullips-updater-config'}}, {'name': 'environment', 'secret': {'secretName': 'tullips-updater-env'}}]
        resources.append({'apiVersion': 'apps/v1', 'kind': 'Deployment', 'metadata': {'name': 'tullips-' + name, 'namespace': namespace}, 'spec': {'replicas': 0, 'strategy': {'type': 'Recreate'}, 'selector': {'matchLabels': {'app': 'tullips-' + name}}, 'template': {'metadata': {'labels': labels}, 'spec': spec}}})
        resources.append(resource('Service', 'tullips-' + name, spec={'selector': {'app': 'tullips-' + name}, 'ports': [{'port': port, 'targetPort': port}]}))
    return resources


def units(c):
    project = Path(c['project_dir'])
    for name in ('api', 'worker', 'web', 'updater'):
        if name == 'updater':
            command = f'{sys.executable} /opt/tullips-updater/update.py --config {c["config_path"]} serve'
            directory, user = '/opt/tullips-updater', 'root'
        else:
            command = str(project / 'current' / 'bin' / name) if name != 'web' else f'{shutil.which("node")} .output/server/index.mjs'
            directory = str(project / 'current' / ('web' if name == 'web' else 'backend'))
            user = 'tullips'
        text = f'''[Unit]
Description=Tullips {name}
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User={user}
WorkingDirectory={directory}
EnvironmentFile={c['env_file']}
ExecStart={command}
Restart=on-failure
RestartSec=5
TimeoutStopSec=330
KillSignal=SIGTERM
[Install]
WantedBy=multi-user.target
'''
        Path('/etc/systemd/system/tullips-' + name + '.service').write_text(text)
    run(['systemctl', 'daemon-reload'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['kubernetes', 'systemd', 'coolify'])
    parser.add_argument('--version', default=(ROOT / 'VERSION').read_text().strip())
    parser.add_argument('--env-file', required=True)
    parser.add_argument('--directory', default='/opt/tullips')
    parser.add_argument('--namespace', default='tullips')
    parser.add_argument('--backup-storage', default='20Gi')
    parser.add_argument('--coolify-url')
    parser.add_argument('--coolify-project')
    parser.add_argument('--coolify-server')
    parser.add_argument('--coolify-environment', default='production')
    parser.add_argument('--applications', help='JSON file mapping api/web/worker to existing Coolify application UUIDs')
    args = parser.parse_args()
    os.umask(0o077)
    if args.mode != 'kubernetes' and (sys.platform != 'linux' or os.geteuid() != 0):
        raise ValueError('Run this installer as root on the Linux application server')
    project = Path(args.directory).resolve()
    project.mkdir(parents=True, exist_ok=True, mode=0o755)
    state = project / '.tullips-updater'
    state.mkdir(exist_ok=True, mode=0o700)
    config_path = state / 'updater.json'
    if config_path.exists(): raise ValueError('This installation is already configured; use its updater CLI')
    environment = env_file(args.env_file)
    for key in ('DATABASE_URL', 'APP_URL', 'BETTER_AUTH_SECRET', 'INTERNAL_API_SECRET', 'ENCRYPTION_KEY'):
        if not environment.get(key): raise ValueError(f'{key} is required in the environment file')
    environment.setdefault('UPDATER_TOKEN', secrets.token_urlsafe(36))
    environment.setdefault('DEPLOYMENT_MODE', 'selfhosted')
    environment.setdefault('BETTER_AUTH_URL', environment['APP_URL'])
    environment.setdefault('API_URL', 'http://127.0.0.1:8080')
    environment.setdefault('UPDATER_URL', 'http://127.0.0.1:8090')
    environment.setdefault('HOST', '127.0.0.1')
    environment.setdefault('PORT', '3000')
    if args.mode == 'systemd':
        environment.setdefault('API_BIND', '127.0.0.1:8080')
        environment.setdefault('WORKER_BIND', '127.0.0.1:8081')
    runtime_env = state / 'environment'
    runtime_env.write_text('\n'.join(f'{k}={v}' for k,v in environment.items())+'\n')
    c = {'mode': args.mode, 'repository': 'jcardonne/tullips', 'version': args.version, 'state_dir': str(state),
         'project_dir': str(project), 'env_file': str(runtime_env), 'config_path': str(config_path), 'namespace': args.namespace,
         'retention': 3, 'api_url': 'http://127.0.0.1:8080/healthz', 'web_url': 'http://127.0.0.1:3000/healthz', 'worker_url': 'http://127.0.0.1:8081/healthz'}
    m = signed_release(c['repository'], args.version, state / 'downloads' / args.version)
    if args.mode == 'coolify':
        if not all((args.coolify_url, environment.get('COOLIFY_TOKEN'))): raise ValueError('Coolify URL and COOLIFY_TOKEN are required')
        c['coolify_url'] = args.coolify_url
        c['applications'] = read(args.applications) if args.applications else {}
    atomic(config_path, c)
    u = Updater(config_path)
    if args.mode == 'kubernetes':
        run(['kubectl', 'create', 'namespace', args.namespace, '--dry-run=client', '-o', 'json'], stdout=open(state / 'namespace.json', 'wb'))
        run(['kubectl', 'apply', '-f', state / 'namespace.json'])
        # Refuse to reset replicas on an existing install. Existing installs first export
        # their deployment/configuration and adopt these names during the migration.
        existing = u.deployment.kube('get', 'deployments', '-l', 'app=tullips-api', '-o', 'json')
        if json.loads(existing)['items']: raise ValueError('Existing Kubernetes deployment detected; use the documented adoption procedure')
        resources = kube_resources(c, m, environment, args.backup_storage)
        u.deployment.kube('apply', '-f', '-', input=json.dumps({'apiVersion':'v1','kind':'List','items':resources}).encode())
        u.save(previous=u.deployment.snapshot())
        u.deployment.migrate(m)
        for name in ('api','web','worker','updater'):
            u.deployment.kube('scale', 'deployment/tullips-' + name, '--replicas=1')
            u.deployment.kube('rollout', 'status', 'deployment/tullips-' + name, '--timeout=180s')
    elif args.mode == 'systemd':
        if not shutil.which('node') or not shutil.which('go'): raise ValueError('Install Node.js and Go before installing from source')
        run(['sh', '-c', 'id tullips >/dev/null 2>&1 || useradd --system --create-home tullips'])
        shutil.copytree(ROOT / 'updater', '/opt/tullips-updater', dirs_exist_ok=True)
        os.chmod('/opt/tullips-updater', 0o755)
        u.deployment.prepare(m)
        u.deployment.migrate(m)
        units(c)
        u.deployment.switch_source(project / 'releases' / args.version)
        run(['systemctl', 'enable', '--now', 'tullips-api', 'tullips-web', 'tullips-worker', 'tullips-updater'])
        u.deployment.healthy()
    else:
        if not c['applications']:
            if not all((args.coolify_project, args.coolify_server)): raise ValueError('Provide the Coolify project and server UUIDs')
            for component, port in (('api',8080),('web',3000),('worker',8081)):
                image = m['images']['web' if component == 'web' else 'backend']
                name, digest = image.split('@sha256:')
                app = u.deployment.coolify('applications/dockerimage', 'POST', {'project_uuid': args.coolify_project,
                    'server_uuid': args.coolify_server, 'environment_name': args.coolify_environment,
                    'name': 'tullips-' + component, 'docker_registry_image_name': name, 'docker_registry_image_tag': 'sha256-' + digest,
                    'ports_exposes': str(port), 'ports_mappings': f'127.0.0.1:{port}:{port}', 'health_check_enabled': True,
                    'health_check_path': '/healthz', 'start_command': '/app/' + component if component != 'web' else 'npm start',
                    'custom_docker_run_options': '--stop-timeout=330 --add-host=host.docker.internal:host-gateway', 'instant_deploy': False})
                c['applications'][component] = app['uuid']
                # Host URLs must be reachable from the managed application containers.
                app_env = environment | {'HOST':'0.0.0.0', 'API_URL':'http://host.docker.internal:8080', 'UPDATER_URL':'http://host.docker.internal:8090'}
                app_env.pop('COOLIFY_TOKEN', None)
                u.deployment.coolify('applications/' + app['uuid'] + '/envs/bulk', 'PATCH', {'data':[{'key': k, 'value': v, 'is_buildtime': False, 'is_runtime': True} for k,v in app_env.items()]})
                atomic(config_path, c)
        bridge = json.loads(run(['docker','network','inspect','bridge']))[0]['IPAM']['Config'][0]['Gateway']
        c['listen'] = bridge + ':8090'
        atomic(config_path, c)
        u.c['applications'] = c['applications']
        if args.applications:
            u.save(previous=u.deployment.snapshot(), previous_version=args.version, phase='adopting')
            u.deployment.stop()
            u.backup()
        u.deployment.prepare(m)
        u.deployment.migrate(m)
        u.deployment.deploy(m)
        u.deployment.healthy()
        shutil.copytree(ROOT / 'updater', '/opt/tullips-updater', dirs_exist_ok=True)
        # Only install the updater unit; Coolify owns application service lifecycles.
        text = f'[Unit]\nDescription=Tullips updater\nAfter=network-online.target\n[Service]\nExecStart={sys.executable} /opt/tullips-updater/update.py --config {config_path} serve\nRestart=on-failure\n[Install]\nWantedBy=multi-user.target\n'
        Path('/etc/systemd/system/tullips-updater.service').write_text(text)
        run(['systemctl','daemon-reload'])
        run(['systemctl','enable','--now','tullips-updater'])
    print('Installed. Register the first user to become installation administrator.')


if __name__ == '__main__':
    from update import safe_error
    try: main()
    except Exception as exc:
        print(safe_error(exc), file=sys.stderr)
        raise SystemExit(1)
