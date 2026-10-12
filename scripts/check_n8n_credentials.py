"""Run on the server over SSH. Output presence checks only; never secret values."""
import json
import shlex
import subprocess

DB = "postgresql-fwcmxt6ry1vt7p5scuhzwizj"
APP = "n8n-fwcmxt6ry1vt7p5scuhzwizj"
SQL = """SELECT json_build_object('draft',w.nodes,'live',h.nodes)
FROM workflow_entity w JOIN workflow_history h
ON h."versionId"=w."activeVersionId"
WHERE w.id='SLffBV8YkETQcXcF';"""
command = 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c ' + shlex.quote(SQL)
workflow = json.loads(subprocess.check_output(
    ["docker", "exec", DB, "sh", "-c", command], text=True))
inventory_command = 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c ' + shlex.quote("SELECT COALESCE(json_agg(id), '[]'::json) FROM credentials_entity;")
ids = json.loads(subprocess.check_output(["docker", "exec", DB, "sh", "-c", inventory_command], text=True))
checker = r"""
const fs = require('fs');
const cp = require('child_process');
const path = require('path');
const dir = fs.mkdtempSync('/dev/shm/groceries-credentials-');
fs.chmodSync(dir, 0o700);
const results=[];
try {
  for (const id of JSON.parse(process.argv[1])) {
    const file=path.join(dir,'credential.json');
    const run=cp.spawnSync('n8n', ['export:credentials','--id='+id,
       '--decrypted','--output='+file], {encoding:'utf8',timeout:60000});
    if (run.status!==0 || !fs.existsSync(file)) {
      results.push({id,retrievable:false}); continue;
    }
    fs.chmodSync(file,0o600);
    const rows=JSON.parse(fs.readFileSync(file,'utf8'));
    for (const row of rows) {
      const d=typeof row.data==='string' ? JSON.parse(row.data) : row.data;
      const oauth=d.oauthTokenData || {};
      results.push({id:row.id,type:row.type,retrievable:true,
        populatedFields:Object.keys(d).filter(k=>d[k]!==null && d[k]!==''),
        apiKeyPresent:!!d.apiKey,accessTokenPresent:!!d.accessToken,
        oauthAccessTokenPresent:!!oauth.access_token,
        oauthRefreshTokenPresent:!!oauth.refresh_token,
        oauthClientIdPresent:!!d.clientId,
        oauthClientSecretPresent:!!d.clientSecret});
    }
    fs.unlinkSync(file);
  }
} finally { fs.rmSync(dir,{recursive:true,force:true}); }
process.stdout.write(JSON.stringify(results));
"""
run = subprocess.run(["docker", "exec", "-u", "node", APP, "node", "-e",
                      checker, json.dumps(ids)], capture_output=True, text=True)
if run.returncode:
    print(json.dumps({"inspection_failed": True, "exit_code": run.returncode}))
else:
    print(json.dumps({"credentials": json.loads(run.stdout),
        "inline_telegram_token_present": any(
            'api.telegram.org/bot' in str(n.get('parameters', {}).get('url', ''))
            for n in workflow['live']),
        "retained_plaintext_exports": False}, indent=2))
