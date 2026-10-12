import datetime,json,sqlite3,subprocess,urllib.request,time
app='1uqewh4rudhzcxwbvg8suqkq';chat=-5138495076
names=[x for x in subprocess.check_output(['docker','ps','--format','{{.Names}}'],text=True).splitlines() if app in x];assert len(names)==1
v=json.loads(subprocess.check_output(['docker','inspect',names[0]],text=True))[0]
env=dict(x.split('=',1) for x in v['Config']['Env']);assert env['RUN_MODE']=='active'
url='https://api.telegram.org/bot'+env['TELEGRAM_BOT_TOKEN']+'/getMe'
with urllib.request.urlopen(url,timeout=20) as r:bot=json.load(r)['result']
path=next(m['Source'] for m in v['Mounts'] if m['Destination']=='/data')+'/groceriesbot.sqlite3'
db=sqlite3.connect(path,timeout=10)
before=db.execute('SELECT count(*) FROM groceries').fetchone()[0]
for uid,command in [(-1001,'/list'),(-1002,'/usage')]:
 seq=db.execute('SELECT coalesce(max(intake_seq),0)+1 FROM telegram_updates').fetchone()[0]
 data={'id':uid,'chat_id':chat,'user_id':bot['id'],'text':command,'message_id':0}
 db.execute('INSERT INTO telegram_updates(update_id,chat_id,user_id,payload_json,received_at,intake_seq) VALUES(?,?,?,?,?,?)',(uid,chat,bot['id'],json.dumps(data),datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),seq))
 db.commit()
 deadline=time.monotonic()+90
 while True:
  state=db.execute('SELECT state FROM telegram_updates WHERE update_id=?',(uid,)).fetchone()[0]
  replies=db.execute('SELECT state,telegram_message_id FROM outbox WHERE update_id=? ORDER BY sequence',(uid,)).fetchall()
  if state=='completed' and replies and all(x[0]=='sent' for x in replies):break
  if time.monotonic()>deadline:raise RuntimeError('smoke command delivery not complete: '+command)
  time.sleep(1)
 print(json.dumps({'administrative_inbox_command':command,'delivered_message_ids':[x[1] for x in replies]}),flush=True)
assert db.execute('SELECT count(*) FROM groceries').fetchone()[0]==before
assert db.execute("SELECT count(*) FROM conversation_messages WHERE content='synthetic persistence marker'").fetchone()[0]==0
usage=db.execute('SELECT sum(cost_micros),count(*) FROM llm_usage').fetchone()
print(json.dumps({'bot_username':bot['username'],'rows_unchanged':before,'model_cost_usd':(usage[0] or 0)/1000000,'model_requests':usage[1]}))
db.close()
