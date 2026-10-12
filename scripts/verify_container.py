#!/usr/bin/env python3
"""Synthetic persistence, instance-lock and container-policy check; network disabled."""
import json,subprocess,time,uuid
image='localhost/groceriesbot:verify';tag='groceries-verify-'+uuid.uuid4().hex[:8];volume=tag+'-data';first=tag+'-first';second=tag+'-second'
def run(*cmd,check=True):return subprocess.run(cmd,text=True,capture_output=True,check=check)
def start(name):run('podman','run','-d','--name',name,'--network','none','--read-only','--memory','128m','--cpus','0.5','--pids-limit','64','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp:size=8m','-v',volume+':/data',image)
def healthy(name):
 for _ in range(50):
  if run('podman','exec',name,'/app/groceriesbot','healthcheck',check=False).returncode==0:return
  time.sleep(.1)
 raise RuntimeError('container health failed')
def private_sql(code):
 mount=run('podman','volume','inspect',volume,'--format','{{.Mountpoint}}').stdout.strip()
 return run('podman','unshare','python3','-c',code,mount+'/groceriesbot.sqlite3').stdout
try:
 start(first);healthy(first);info=json.loads(run('podman','inspect',first).stdout)[0]
 assert info['HostConfig']['ReadonlyRootfs'] and info['HostConfig']['Memory']==134217728
 assert info['Config']['User']=='10001:10001'
 run('podman','stop','--time','10',first);run('podman','rm',first)
 private_sql("import sqlite3,sys;d=sqlite3.connect(sys.argv[1]);d.execute(\"INSERT INTO groceries(chat_id,name,category,quantity,added_at) VALUES(-123,'fixture milk','Dairy',2,'2026-01-01T00:00:00Z')\");d.execute(\"INSERT INTO conversation_messages(chat_id,user_id,role,content,created_at) VALUES(-123,8,'user','fixture memory','2026-01-01T00:00:00Z')\");d.execute(\"UPDATE settings SET value_json='[\\\"Fixture section\\\"]' WHERE key='list.category_order'\");d.commit();d.close()")
 start(first);healthy(first)
 result=private_sql("import sqlite3,sys,json;d=sqlite3.connect(sys.argv[1]);print(json.dumps([d.execute('SELECT count(*) FROM groceries').fetchone()[0],d.execute('SELECT count(*) FROM conversation_messages').fetchone()[0],d.execute(\"SELECT value_json FROM settings WHERE key='list.category_order'\").fetchone()[0]]));d.close()")
 assert json.loads(result)==[1,1,'["Fixture section"]'],result
 start(second);code=int(run('podman','wait',second).stdout.strip());assert code!=0
 logs=run('podman','logs',second).stdout+run('podman','logs',second).stderr;assert 'holds the data lock' in logs
 run('podman','exec',first,'/app/groceriesbot','doctor');assert run('podman','exec',first,'/app/groceriesbot','settings','get','llm.enabled').stdout.strip()=='false'
 print(json.dumps({'network':'disabled','uid':10001,'readonly_root':True,'memory_limit_mib':128,'persistence':'groceries, memory and settings survived recreation','overlap':'second instance rejected','ai_enabled':False}))
finally:
 for name in [first,second]:run('podman','rm','-f',name,check=False)
 run('podman','volume','rm',volume,check=False)
