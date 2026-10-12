#!/usr/bin/env python3
"""Verify a final private import: source equality, counts, integrity and relationships."""
import argparse,hashlib,json,sqlite3,zlib
from pathlib import Path
from import_sheets import dumps,summary
p=argparse.ArgumentParser();p.add_argument('snapshot',type=Path);p.add_argument('database',type=Path);a=p.parse_args();books=json.loads(a.snapshot.read_text());db=sqlite3.connect(a.database.as_uri()+'?mode=ro',uri=True)
try:
 row=db.execute('SELECT digest,payload_zlib FROM source_snapshots').fetchone();assert row is not None
 payload=dumps(books).encode();assert hashlib.sha256(payload).hexdigest()==row[0];assert zlib.decompress(row[1])==payload
 assert db.execute('PRAGMA integrity_check').fetchone()[0]=='ok';assert db.execute('PRAGMA foreign_key_check').fetchall()==[]
 result=summary(db);assert result['issues']==0,'import issues require review';assert result['spreadsheets']==len(books)
 result.update({'source_snapshot_equal':True,'integrity':'ok','foreign_key_issues':0});print(json.dumps(result,indent=2))
finally:db.close()
