#!/usr/bin/env python3
"""Offline native database snapshots. Restore only into an empty destination.

Requires Python 3 and native mysqldump/mysql or pg_dump/pg_restore/psql
clients for the configured engines. ClickHouse uses its HTTP Native format.
Passwords can be provided via password_env in the private configuration file.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import sys
import urllib.parse
import urllib.request


def password(config):
    if config.get('password_env'):
        return os.environ[config['password_env']]
    return config.get('password', '')


def client(config, program, extra=(), stdin=None, output=None):
    env = os.environ.copy()
    executable = config.get('bin_dir', '')
    executable = str(Path(executable) / program) if executable else program
    if config['engine'] == 'mysql':
        env['MYSQL_PWD'] = password(config)
        command = [executable, '--user='+config.get('user', 'root')]
        if config.get('socket'):
            command += ['--socket='+config['socket']]
        else:
            command += ['--host='+config.get('host', '127.0.0.1'), '--port='+str(config.get('port', 3306)), '--protocol=TCP']
        command += list(extra) + [config['database']]
    else:
        env.update(PGPASSWORD=password(config), PGHOST=config.get('host', '127.0.0.1'), PGPORT=str(config.get('port', 5432)), PGUSER=config['user'], PGDATABASE=config['database'])
        if config.get('sslmode'):
            env['PGSSLMODE'] = config['sslmode']
        command = [executable] + list(extra)
    if config.get('client_container'):
        forwarded = ['MYSQL_PWD'] if config['engine'] == 'mysql' else ['PGPASSWORD','PGHOST','PGPORT','PGUSER','PGDATABASE','PGSSLMODE']
        command = ['docker','exec','-i'] + [item for key in forwarded if key in env for item in ('--env',key)] + [config['client_container']] + command
    result = subprocess.run(command, env=env, stdin=stdin, stdout=output or subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError(program+' failed: '+result.stderr.decode(errors='replace'))
    return result.stdout


def clickhouse(config, query, body=None):
    endpoint = config['url'].rstrip('/')
    if urllib.parse.urlparse(endpoint).scheme not in ('http', 'https'):
        raise ValueError('ClickHouse url must use http or https')
    endpoint += '/?' + urllib.parse.urlencode({'database':config['database'], 'query':query})
    auth = base64.b64encode((config.get('user', 'default')+':'+password(config)).encode()).decode()
    request = urllib.request.Request(endpoint, data=body if body is not None else b'', headers={'Authorization':'Basic '+auth}, method='POST')
    return urllib.request.urlopen(request, timeout=3600)


def tables(config):
    engine = config['engine']
    if engine == 'sqlite':
        file = Path(config['path'])
        if not file.exists():
            return []
        with sqlite3.connect('file:'+str(file.resolve())+'?mode=ro', uri=True) as db:
            return [row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")]
    if engine == 'mysql':
        return client(config,'mysql',['--batch','--skip-column-names','-e','SHOW TABLES']).decode().splitlines()
    if engine == 'postgres':
        return client(config,'psql',['-XAt','-v','ON_ERROR_STOP=1','-c',"SELECT tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')"]).decode().splitlines()
    if engine == 'clickhouse':
        with clickhouse(config, 'SHOW TABLES FORMAT JSON') as response:
            return [row['name'] for row in json.load(response)['data']]
    raise ValueError('Unsupported engine: '+engine)


def copy_stream(source, destination):
    while True:
        block = source.read(1024*1024)
        if not block:
            return
        destination.write(block)


def digest(file):
    value = hashlib.sha256()
    with file.open('rb') as stream:
        for block in iter(lambda:stream.read(1024*1024), b''):
            value.update(block)
    return value.hexdigest()


def backup(config, folder):
    folder.mkdir(mode=0o700, parents=True, exist_ok=False)
    manifest = {'format':1, 'databases':{}, 'files':{}}
    for role, database in config.items():
        if role not in ('main','log'):
            raise ValueError('Configuration accepts only main and optional log')
        engine = database['engine']
        names = tables(database)
        manifest['databases'][role] = {'engine':engine, 'tables':names}
        prefix = folder / role
        if engine == 'sqlite':
            with sqlite3.connect('file:'+str(Path(database['path']).resolve())+'?mode=ro', uri=True) as source, sqlite3.connect(str(prefix)+'.sqlite') as destination:
                source.backup(destination)
        elif engine == 'mysql':
            args = ['--single-transaction','--skip-lock-tables','--routines','--triggers','--events','--hex-blob','--set-gtid-purged=OFF']
            executable = str(Path(database.get('bin_dir',''))/'mysqldump') if database.get('bin_dir') else 'mysqldump'
            version_command = [executable,'--version']
            if database.get('client_container'):
                version_command = ['docker','exec',database['client_container']] + version_command
            version = subprocess.check_output(version_command, text=True)
            if re.search(r'(?:Ver|Distrib)\s+(?:8|9)\.',version):
                args.append('--column-statistics=0')
            with Path(str(prefix)+'.sql').open('wb') as output:
                client(database,'mysqldump',args,output=output)
        elif engine == 'postgres':
            with Path(str(prefix)+'.dump').open('wb') as output:
                client(database,'pg_dump',['--format=custom','--no-owner','--no-acl'],output=output)
        elif engine == 'clickhouse':
            for index, name in enumerate(names):
                identifier = '`'+name.replace('`','``')+'`'
                with clickhouse(database, 'SHOW CREATE TABLE '+identifier+' FORMAT JSON') as response:
                    ddl = json.load(response)['data'][0]['statement']
                Path(str(prefix)+f'-{index}.ddl').write_text(ddl, encoding='utf8')
                with clickhouse(database, 'SELECT * FROM '+identifier+' FORMAT Native') as response, Path(str(prefix)+f'-{index}.native').open('wb') as output:
                    copy_stream(response,output)
    for file in folder.iterdir():
        file.chmod(0o600)
        manifest['files'][file.name] = digest(file)
    (folder/'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n', encoding='utf8')
    (folder/'manifest.json').chmod(0o600)
    print('Snapshot complete:',folder)


def verify(folder):
    manifest = json.loads((folder/'manifest.json').read_text())
    if manifest.get('format') != 1:
        raise ValueError('Unsupported snapshot format')
    for name, expected in manifest['files'].items():
        if Path(name).name != name or digest(folder/name) != expected:
            raise ValueError('Snapshot checksum mismatch: '+name)
    return manifest


def restore(config, folder):
    manifest = verify(folder)
    if set(config) != set(manifest['databases']):
        raise ValueError('Restore configuration must include exactly the snapshotted databases')
    # Inspect every destination before creating any tables. Never overwrite a
    # live database; switch the application DSNs after successful verification.
    for role, database in config.items():
        if database['engine'] != manifest['databases'][role]['engine']:
            raise ValueError('Engine mismatch for '+role)
        if tables(database):
            raise ValueError(role+' destination must be empty; restore into a new database')
    for role, database in config.items():
        prefix = folder / role
        engine = database['engine']
        if engine == 'sqlite':
            with sqlite3.connect('file:'+str(prefix)+'.sqlite?mode=ro', uri=True) as source, sqlite3.connect(database['path']) as destination:
                source.backup(destination)
        elif engine == 'mysql':
            with Path(str(prefix)+'.sql').open('rb') as source:
                client(database,'mysql',stdin=source)
        elif engine == 'postgres':
            with Path(str(prefix)+'.dump').open('rb') as source:
                client(database,'pg_restore',['--exit-on-error','--no-owner','--no-acl','--dbname='+database['database']],stdin=source)
        elif engine == 'clickhouse':
            for index, name in enumerate(manifest['databases'][role]['tables']):
                identifier = '`'+name.replace('`','``')+'`'
                ddl = Path(str(prefix)+f'-{index}.ddl').read_text()
                # SHOW CREATE qualifies the original database. Only replace
                # its CREATE target, preserving engine, columns and sorting.
                ddl = re.sub(r'^CREATE TABLE\s+\S+', 'CREATE TABLE '+identifier, ddl, count=1)
                with clickhouse(database,ddl):
                    pass
                with Path(str(prefix)+f'-{index}.native').open('rb') as source:
                    with clickhouse(database,'INSERT INTO '+identifier+' FORMAT Native',body=source):
                        pass
        if sorted(tables(database)) != sorted(manifest['databases'][role]['tables']):
            raise ValueError('Restored table inventory mismatch for '+role)
    print('Restore complete. Verify application data before switching DSNs:',folder)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['backup','verify','restore'])
    parser.add_argument('--config', type=Path)
    parser.add_argument('--snapshot', type=Path, required=True)
    parser.add_argument('--offline', action='store_true', help='Confirm all application writers and background workers have stopped')
    args = parser.parse_args()
    os.umask(0o077)
    if args.action == 'verify':
        verify(args.snapshot)
        print('Snapshot checksums verified')
        return
    if not args.offline or not args.config:
        parser.error('backup/restore require --offline and --config')
    config = json.loads(args.config.read_text())
    if 'main' not in config:
        parser.error('configuration requires main')
    identities = []
    for database in config.values():
        if database['engine'] == 'sqlite':
            identity = ('sqlite',str(Path(database['path']).resolve()))
        else:
            identity = (database['engine'],database.get('client_container',''),database.get('host',database.get('url','127.0.0.1')),database.get('port'),database['database'])
        if identity in identities:
            parser.error('main/log refer to the same database; omit log when it shares main')
        identities.append(identity)
    if args.action == 'backup':
        backup(config,args.snapshot)
    else:
        restore(config,args.snapshot)

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('Snapshot operation failed:',error,file=sys.stderr)
        sys.exit(1)
