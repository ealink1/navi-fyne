#!/usr/bin/env python3
"""Start the real desktop app with an isolated workspace, without a browser."""
import argparse
import json
import os
from pathlib import Path
import signal
import shutil
import plistlib
import sqlite3
import subprocess
import tempfile
import time
import uuid

ROOT = Path(__file__).resolve().parent.parent


def stop_process(process):
    if process.poll() is None:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=20)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=10)


def stop_pid(pid):
    if not pid:
        return
    try:
        os.kill(pid, signal.SIGTERM)
        deadline = time.monotonic()+20
        while time.monotonic() < deadline:
            os.kill(pid, 0)
            time.sleep(0.1)
        os.kill(pid, signal.SIGKILL)
    except ProcessLookupError:
        return


def verify_upgrade(directory, root, old_process, executable):
    source = executable.parents[2]
    if source.suffix != '.app':
        raise RuntimeError('upgrade smoke check currently requires a macOS .app')
    token = str(uuid.uuid4())
    target = source
    stage = directory/('.navi-fyne-stage-'+token)
    shutil.copytree(source, stage)
    next_version = '0.1.1'
    subprocess.run(['go','build','-trimpath','-ldflags',f'-s -w -X main.version={next_version}',
                    '-o',str(stage/'Contents/MacOS/navi-fyne'),'./cmd/navi-fyne'],cwd=ROOT,check=True)
    marker_path = stage/'Contents/Resources/navi-fyne.package.json'
    marker = json.loads(marker_path.read_text());marker['version'] = next_version
    marker_path.write_text(json.dumps(marker))
    info_path = stage/'Contents/Info.plist'
    info = plistlib.loads(info_path.read_bytes())
    info['CFBundleShortVersionString'] = info['CFBundleVersion'] = next_version
    info_path.write_bytes(plistlib.dumps(info))
    updates = root/'updates';updates.mkdir(mode=0o700)
    health = updates/('health-'+token+'.json')
    backup = str(target)+'.backup-'+token
    report = updates/'last-update.json'
    request = {'target':str(target),'stage':str(stage),'backup':backup,'parentPid':old_process.pid,
               'version':next_version,'dataRoot':str(root),'healthFile':str(health),'token':token,
               'report':str(report),'stateBackup':str(updates/('state-'+token+'.sqlite'))}
    request_file = directory/'request.json';request_file.write_text(json.dumps(request));request_file.chmod(0o600)
    with sqlite3.connect(root/'state.sqlite') as database:
        database.execute("INSERT OR REPLACE INTO settings(key,value) VALUES('native-selfcheck','before')")
    helper = ROOT/'bin/update-helper'
    with (directory/'helper.log').open('w+') as log:
        process = subprocess.Popen([str(helper),'--request',str(request_file)],stdout=log,stderr=log)
        child_pid = None
        try:
            stop_process(old_process)
            process.wait(timeout=70)
            receipt = json.loads(report.read_text()) if report.exists() else {}
            child_pid = receipt.get('processId')
            if process.returncode != 0 or not receipt.get('success'):
                log.seek(0)
                raise RuntimeError('native update helper failed: '+log.read()[-3000:])
            if receipt.get('version') != next_version or not child_pid:
                raise RuntimeError('new native application health handshake mismatch')
            os.kill(child_pid,0)
            installed_version = subprocess.check_output([str(target/'Contents/MacOS/navi-fyne'),'--version'],text=True).strip()
            if installed_version != next_version:
                raise RuntimeError('installed executable does not match the new version')
            if not Path(backup).is_dir() or not Path(request['stateBackup']).is_file():
                raise RuntimeError('successful update lost application/state backups')
            with sqlite3.connect(request['stateBackup']) as database:
                value = database.execute("SELECT value FROM settings WHERE key='native-selfcheck'").fetchone()
                if value != ('before',):
                    raise RuntimeError('update state snapshot lost WAL data')
            print('Native 0.1.0 → 0.1.1 helper update: parent exit, whole-bundle replacement, '
                  'new event-loop/PID health and retained state/application backups passed.')
        finally:
            if process.poll() is None:
                process.kill();process.wait(timeout=10)
            if not child_pid and health.exists():
                child_pid = json.loads(health.read_text()).get('pid')
            stop_pid(child_pid)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--app', type=Path, default=ROOT/'bin/SuperLink.app/Contents/MacOS/navi-fyne')
    parser.add_argument('--upgrade', action='store_true', help='also build/start a second native version and test the real helper')
    args = parser.parse_args()
    executable = args.app.resolve()
    version = subprocess.check_output([str(executable), '--version'], text=True).strip()
    cache = ROOT/'.cache'
    cache.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='native-smoke-', dir=cache) as temporary:
        directory = Path(temporary)
        startup_executable = executable
        if args.upgrade:
            copied_app = directory/'SuperLink.app'
            shutil.copytree(executable.parents[2], copied_app)
            startup_executable = copied_app/'Contents/MacOS/navi-fyne'
        root = directory/'workspace'
        health = directory/'health.json'
        token = str(uuid.uuid4())
        with (directory/'native.log').open('w+') as log:
            process = subprocess.Popen([str(startup_executable), '--data-root', str(root),
                                        '--health-file', str(health), '--health-token', token],
                                       stdout=log, stderr=log)
            try:
                deadline = time.monotonic()+45
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        log.seek(0)
                        raise RuntimeError('native app exited during startup: '+log.read()[-3000:])
                    if health.exists():
                        data = json.loads(health.read_text())
                        if data.get('token') == token and data.get('version') == version and data.get('pid') == process.pid:
                            break
                    time.sleep(0.1)
                else:
                    raise RuntimeError('native event loop did not confirm startup health')
                if not (root/'state.sqlite').is_file() or not any((root/'drivers').rglob('*sqlite*')):
                    raise RuntimeError('workspace or bundled SQLite agent missing')
                duplicate = subprocess.run([str(startup_executable), '--data-root', str(root)],
                                           capture_output=True, text=True, timeout=15)
                if duplicate.returncode == 0 or 'already open' not in duplicate.stderr:
                    raise RuntimeError('second native instance was not rejected')
                if args.upgrade:
                    verify_upgrade(directory,root,process,startup_executable)
                else:
                    stop_process(process)
                    if process.returncode != 0:
                        raise RuntimeError('native application did not terminate cleanly')
                print(f'Native app v{version}: event-loop health, offline SQLite installation, '
                      'exclusive workspace and clean shutdown passed.')
            finally:
                if process.poll() is None:
                    stop_process(process)


if __name__ == '__main__':
    main()
