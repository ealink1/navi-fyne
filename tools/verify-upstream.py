#!/usr/bin/env python3
"""Verify retained upstream file hashes; refresh provenance after reviewed edits.

--refresh requires a local checkout at the pinned source commit and also emits
a reviewable patch against the rewritten, gofmt-normalized upstream source.
"""
import argparse
import difflib
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
MANIFEST = ROOT / 'docs/upstream-import.json'
ADDITIONS = {
    **{f'cmd/driver-agent/request_{name}.go': 'cmd/optional-driver-agent/main.go'
       for name in ['catalog', 'changes', 'external', 'lifecycle', 'session', 'sql']},
    'cmd/driver-agent/request_decode.go': None,
    'cmd/driver-agent/request_decode_test.go': None,
    'internal/upstream/db/agent_args_wire.go': None,
    'internal/upstream/db/agent_args_wire_test.go': None,
    'internal/upstream/db/agent_binary_wire.go': None,
    'internal/upstream/db/agent_binary_wire_test.go': None,
    'internal/upstream/db/native_query_values.go': None,
    'internal/upstream/db/dsn_tls_fixture_test.go': None,
    'internal/upstream/sqlparam/fyne_precision_test.go': None,
}
EXCLUDED_PRIVATE_KEYS = [
    'third_party/highgo-pq/certs/postgresql.key',
    'third_party/highgo-pq/certs/server.key',
]


def digest(data):
    return hashlib.sha256(data).hexdigest()


def retained_files():
    folders = ['internal/upstream', 'cmd/driver-agent', 'third_party/highgo-pq',
               'third_party/go-irisnative', 'tools/gen-i18n-catalog-zip']
    files = []
    for folder in folders:
        files.extend(path for path in (ROOT / folder).rglob('*')
                     if path.is_file() and '.git' not in path.parts
                     and path.relative_to(ROOT).as_posix() not in EXCLUDED_PRIVATE_KEYS)
    files.extend([ROOT / 'LICENSE', ROOT / 'testdata/issue-1326-oracle11g.sql'])
    return sorted(files)


def source_path(target):
    relative = target.relative_to(ROOT).as_posix()
    if relative.startswith('internal/upstream/i18n/'):
        return relative.replace('internal/upstream/i18n/', 'shared/i18n/', 1)
    if relative.startswith('internal/upstream/'):
        return relative.replace('internal/upstream/', 'internal/', 1)
    if relative.startswith('cmd/driver-agent/'):
        return relative.replace('cmd/driver-agent/', 'cmd/optional-driver-agent/', 1)
    return relative


def refresh(source):
    spec = importlib.util.spec_from_file_location('upstream_import', ROOT / 'tools/import-upstream.py')
    importer = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(importer)
    actual = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
    if actual != importer.COMMIT:
        raise SystemExit('source must be at the pinned upstream commit')
    source_files = set(subprocess.check_output(['git', 'ls-tree', '-r', '--name-only', actual],
                                             cwd=source, text=True).splitlines())
    records, patch = [], []
    with tempfile.TemporaryDirectory(prefix='navifyne-provenance-') as temporary:
        bases, go_files = {}, []
        for target in retained_files():
            relative = target.relative_to(ROOT).as_posix()
            origin = source_path(target)
            added = origin not in source_files
            if added:
                if relative not in ADDITIONS:
                    raise SystemExit(f'unregistered retained-tree addition: {relative}')
                origin = ADDITIONS[relative]
            raw = subprocess.check_output(['git', 'show', f'{actual}:{origin}'], cwd=source) if origin else b''
            rewritten = (importer.rewrite(raw.decode()).encode() if target.suffix in {'.go', '.json', '.md'} else raw) if not added else b''
            base = Path(temporary) / relative
            base.parent.mkdir(parents=True, exist_ok=True)
            base.write_bytes(rewritten)
            bases[relative] = base
            if target.suffix == '.go' and not added:
                go_files.append(str(base))
            records.append({'source': origin, 'target': relative, 'source_sha256': digest(raw) if origin else None,
                            **({'adaptation': 'split from source' if origin else 'authored addition'} if added else {}),
                            'rewritten_sha256': digest(rewritten), 'imported_sha256': digest(target.read_bytes())})
        for offset in range(0, len(go_files), 80):
            subprocess.run(['gofmt', '-w'] + go_files[offset:offset+80], check=True)
        for record in records:
            relative = record['target']
            if Path(relative).suffix not in {'.go', '.json', '.md'}:
                continue
            before = bases[relative].read_text()
            after = (ROOT / relative).read_text()
            patch.extend(difflib.unified_diff(before.splitlines(keepends=True), after.splitlines(keepends=True),
                                            fromfile='a/' + relative, tofile='b/' + relative))
    MANIFEST.write_text(json.dumps({'schema': 2, 'repository': 'Syngnat/GoNavi', 'commit': actual,
                                   'excluded': ['internal/jvm', 'internal/logger/wails_adapter.go',
                                                'internal/logger/wails_adapter_test.go'] + EXCLUDED_PRIVATE_KEYS,
                                   'files': records}, indent=2) + '\n')
    (ROOT / 'docs/upstream-adaptations.patch').write_text(''.join(patch))


def verify():
    manifest = json.loads(MANIFEST.read_text())
    expected = {record['target'] for record in manifest['files']}
    actual = {path.relative_to(ROOT).as_posix() for path in retained_files()}
    errors = sorted(expected.symmetric_difference(actual))
    for record in manifest['files']:
        target = ROOT / record['target']
        if not target.is_file() or digest(target.read_bytes()) != record['imported_sha256']:
            errors.append(record['target'])
    if errors:
        raise SystemExit('upstream provenance mismatch:\n' + '\n'.join(sorted(set(errors))))
    print(f'Upstream provenance verified: {len(expected)} retained files at {manifest["commit"]}.')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--refresh', type=Path, metavar='PINNED_CHECKOUT')
    args = parser.parse_args()
    if args.refresh:
        refresh(args.refresh.resolve())
    verify()


if __name__ == '__main__':
    main()
