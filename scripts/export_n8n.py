"""Run on the n8n host via SSH; stdout is a private transfer, never a log."""
import json
import re
import shlex
import subprocess
import sys

DB = 'postgresql-fwcmxt6ry1vt7p5scuhzwizj'
APP = 'n8n-fwcmxt6ry1vt7p5scuhzwizj'

def run():
    sql = '''SELECT h.nodes FROM workflow_entity w JOIN workflow_history h
    ON h."versionId"=w."activeVersionId" WHERE w.id='SLffBV8YkETQcXcF';'''
    cmd = 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c ' + shlex.quote(sql)
    nodes = json.loads(subprocess.check_output(['docker','exec',DB,'sh','-c',cmd], text=True))
    ids = sorted({str(c['id']) for n in nodes for c in n.get('credentials',{}).values()})
    exporter = r'''
const fs=require('fs'), cp=require('child_process'), path=require('path');
const dir=fs.mkdtempSync('/dev/shm/groceries-export-'); fs.chmodSync(dir,0o700);
try {
  const file=path.join(dir,'credentials.json');
  const result=cp.spawnSync('n8n',['export:credentials','--all','--decrypted','--output='+file],
    {encoding:'utf8',timeout:90000});
  if(result.status!==0) throw new Error('Credential export failed');
  fs.chmodSync(file,0o600);
  const ids=new Set(JSON.parse(process.argv[1]));
  process.stdout.write(JSON.stringify(JSON.parse(fs.readFileSync(file,'utf8')).filter(c=>ids.has(c.id))));
} finally { fs.rmSync(dir,{recursive:true,force:true}); }
'''
    raw = subprocess.run(['docker','exec','-u','node',APP,'node','-e',exporter,json.dumps(ids)], capture_output=True,text=True,timeout=120)
    if raw.returncode:
        raise RuntimeError('Credential export failed')
    credentials = json.loads(raw.stdout)
    refs = []
    for n in nodes:
        if 'googleSheets' in n['type']:
            p = n['parameters']
            refs.append({key: p.get(key) for key in ('documentId','sheetName')})
    inline = []
    for n in nodes:
        match = re.search(r'api\.telegram\.org/bot([^/\s]+)',n.get('parameters',{}).get('url',''))
        if match:
            inline.append(match.group(1))
    trigger = next(n for n in nodes if 'telegramTrigger' in n['type'])
    print(json.dumps({'credentials':credentials,'sheet_refs':refs,'inline_telegram_tokens':inline,
        'allowed_chat_ids':trigger['parameters'].get('additionalFields',{}).get('chatIds','')}))

if __name__ == '__main__':
    try:
        run()
    except Exception:
        print('n8n export failed; secret-bearing output withheld',file=sys.stderr)
        sys.exit(1)
