#!/usr/bin/env python3
"""Check tracked content against private token values without displaying tokens."""
import json,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[1];values=[]
if (root/'.env').exists():
 for line in (root/'.env').read_text().splitlines():
  if '=' not in line:continue
  key,raw=line.split('=',1)
  if any(word in key for word in ['TOKEN','KEY','SECRET']):
   try:value=json.loads(raw)
   except ValueError:continue
   if isinstance(value,str) and len(value)>12:values.append(value.encode())
paths=subprocess.check_output(['git','ls-files','-z'],cwd=root).split(b'\0');bad=[]
for raw in paths:
 if not raw:continue
 name=raw.decode();p=root/name
 if name=='.env' or name.startswith(('data/','.local/','.worktrees/')):bad.append(name);continue
 if p.is_file() and any(v in p.read_bytes() for v in values):bad.append(name)
if bad:print('Private content found in: '+', '.join(bad));sys.exit(1)
print('Tracked-file secret scan: PASS')
