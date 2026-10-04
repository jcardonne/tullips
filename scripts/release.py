#!/usr/bin/env python3
"""Package the exact Git release and its immutable image references for attestation."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile
import sys

root = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(root / 'updater'))
from update import validate_manifest, version
parser = argparse.ArgumentParser(description=__doc__)
for name in ('version', 'repository', 'backend', 'web', 'updater'):
    parser.add_argument('--' + name, required=True)
args = parser.parse_args()
version(args.version)
if (root / 'VERSION').read_text().strip() != args.version:
    raise SystemExit('VERSION must match the release tag')
policy = json.loads((root / 'release-policy.json').read_text())
output = root / '.release'
output.mkdir(exist_ok=True)
archive = subprocess.check_output(['git', 'archive', '--format=tar', 'HEAD'], cwd=root)
checksums = {}
with tarfile.open(fileobj=io.BytesIO(archive)) as source, tarfile.open(output / 'source.tar.gz', 'w:gz') as target:
    for member in source:
        if not member.isdir() and not member.isfile():
            raise SystemExit('Release archives must not contain links or special files')
        data = source.extractfile(member) if member.isfile() else None
        if data:
            raw = data.read()
            checksums[member.name] = hashlib.sha256(raw).hexdigest()
            target.addfile(member, io.BytesIO(raw))
        else:
            target.addfile(member)
    raw = json.dumps(checksums, sort_keys=True).encode()
    info = tarfile.TarInfo('source-files.json')
    info.size = len(raw)
    info.mode = 0o644
    target.addfile(info, io.BytesIO(raw))
manifest = {
    'format': 1, 'updater_protocol': 1, 'repository': args.repository,
    'version': args.version, **policy,
    'images': {component: f'ghcr.io/{args.repository.lower()}-{component}@{getattr(args, component)}' for component in ('backend', 'web', 'updater')},
    'source_sha256': hashlib.sha256((output / 'source.tar.gz').read_bytes()).hexdigest(),
}
validate_manifest(manifest, args.repository)
(output / 'release.json').write_text(json.dumps(manifest, indent=2) + '\n')
