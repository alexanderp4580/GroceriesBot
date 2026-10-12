#!/usr/bin/env python3
"""Run on the Docker host. Print checked runtime metadata, excluding secrets and row contents."""
import argparse,json,os,sqlite3,subprocess
p=argparse.ArgumentParser();p.add_argument('application_uuid');p.add_argument('--record',required=True);a=p.parse_args()
rows=[json.loads(x) for x in subprocess.check_output(['docker','ps','--format','{{json .}}'],text=True).splitlines()];names=[x['Names'] for x in rows if a.application_uuid in x['Names']];assert len(names)==1,'expected one running instance'
v=json.loads(subprocess.check_output(['docker','inspect',names[0]],text=True))[0];h=v['HostConfig'];c=v['Config'];s=v['State'];mount=next(m for m in v['Mounts'] if m['Destination']=='/data')
assert c['User']=='10001:10001' and h['ReadonlyRootfs'] and h['Memory']==134217728 and h['NanoCpus']==500000000;assert h['PortBindings']=={};assert mount['Type']=='volume' and mount['RW'];assert s.get('Health',{}).get('Status')=='healthy'
subprocess.run(['docker','exec',names[0],'/app/groceriesbot','doctor'],check=True,stdout=subprocess.DEVNULL)
path=mount['Source']+'/groceriesbot.sqlite3';stat=os.stat(path);assert stat.st_uid==10001 and stat.st_mode&0o777==0o600
conn=sqlite3.connect('file:'+path+'?mode=ro',uri=True)
try:
 values={k:json.loads(raw) for k,raw in conn.execute("SELECT key,value_json FROM settings WHERE key IN ('llm.enabled','llm.model','llm.reasoning_effort','llm.daily_budget_usd','llm.monthly_budget_usd','telegram.allowed_chat_ids')")}
 counts={table:conn.execute('SELECT count(*) FROM '+table).fetchone()[0] for table in ['groceries','conversation_messages','telegram_updates','llm_usage']};pending=conn.execute("SELECT count(*) FROM outbox WHERE state='pending'").fetchone()[0]
finally:conn.close()
result={'container':names[0],'image_id':v['Image'],'health':s['Health']['Status'],'user':c['User'],'readonly_root':True,'memory_limit_mib':128,'cpu_limit':.5,'data_volume':mount['Name'],'mount_path':mount['Source'],'database_mode':'0600','settings':values,'counts':counts,'pending_outbox':pending}
with open(a.record,'x') as f:json.dump(result,f,indent=2)
print(json.dumps(result))
