#!/usr/bin/env python3
"""Run on the server with a closed DB on stdin, only after stopping the app."""
import hashlib,os,sqlite3,subprocess,sys
from pathlib import Path
app_uuid,expected=sys.argv[1:]
running=subprocess.check_output(['docker','ps','--format','{{.Names}}'],text=True)
assert app_uuid not in running,'stop the application before replacing storage'
volume=app_uuid+'_groceriesbot-data'
root=Path(subprocess.check_output(['docker','volume','inspect','--format','{{.Mountpoint}}',volume],text=True).strip())
temporary=root/'import.sqlite3'
with temporary.open('xb') as out:
 while chunk:=sys.stdin.buffer.read(1024*1024):out.write(chunk)
 out.flush();os.fsync(out.fileno())
assert hashlib.sha256(temporary.read_bytes()).hexdigest()==expected,'transfer digest mismatch'
db=sqlite3.connect('file:'+str(temporary)+'?mode=ro',uri=True)
assert db.execute('PRAGMA integrity_check').fetchone()[0]=='ok'
assert db.execute('PRAGMA foreign_key_check').fetchall()==[]
assert db.execute("SELECT value_json FROM settings WHERE key='llm.enabled'").fetchone()[0]=='true'
count=db.execute('SELECT count(*) FROM groceries').fetchone()[0]
db.close()
for suffix in ['-wal','-shm']:
 (root/('groceriesbot.sqlite3'+suffix)).unlink(missing_ok=True)
os.chown(temporary,10001,10001);temporary.chmod(0o600)
os.replace(temporary,root/'groceriesbot.sqlite3')
fd=os.open(root,os.O_DIRECTORY);os.fsync(fd);os.close(fd)
print('Installed verified database; grocery rows:',count)
