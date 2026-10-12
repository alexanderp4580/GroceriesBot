#!/usr/bin/env python3
"""Opt-in model recordings from the real processor in an isolated fixture DB."""
import argparse,datetime,hashlib,json,os,sqlite3,subprocess,sys,urllib.request
from pathlib import Path
from fetch_sheets import env_file
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--list',action='store_true');p.add_argument('--record',type=Path);p.add_argument('--allow-paid-model-calls',action='store_true');p.add_argument('--max-usd',type=float,default=.25);p.add_argument('--scenario',action='append',default=[]);p.add_argument('--resume',type=Path,help='Resume copied synthetic DBs from a prior capture, skipping initial prompts');a=p.parse_args()
cases=json.loads((ROOT/'testdata/llm-scenarios.json').read_text());ids={c['id'] for c in cases}
if set(a.scenario)-ids:p.error('unknown scenario selected')
selected=[c for c in cases if not a.scenario or c['id'] in a.scenario]
if a.list:
 for c in selected:print(c['id']+': '+c['find_out'])
 sys.exit(0)
if not a.allow_paid_model_calls or not a.record or not 0<a.max_usd<=.25:p.error('capture requires an approval flag, new record path, and budget at most $0.25')
if a.resume and not a.scenario:p.error('resume requires explicit scenario selection')
os.umask(0o077);record=a.record.resolve();record.relative_to(ROOT);record.mkdir(parents=True,exist_ok=False)
try:
 values=env_file(ROOT/'.env');model='anthropic/claude-haiku-5.5';resume_info=[];start_ids={}
 if a.resume:
  source=a.resume.resolve()
  # Copy only synthetic capture DBs. Keep the original recorded state immutable.
  original=json.loads((source/'observations.json').read_text());original_ids={s['scenario'] for s in original['scenarios']}
  if set(a.scenario)-original_ids:raise ValueError('selected source scenario is missing')
  for c in selected:
   path=source/(c['id']+'.sqlite3');before=hashlib.sha256(path.read_bytes()).hexdigest()
   src=sqlite3.connect(path.as_uri()+'?mode=ro',uri=True);dst=sqlite3.connect(record/path.name)
   try:
    chats=src.execute('SELECT DISTINCT chat_id FROM telegram_updates').fetchall()
    if not chats or any(row[0]!=-123 for row in chats):raise ValueError('source is not a synthetic capture DB')
    src.backup(dst);start_ids[c['id']]=dst.execute('SELECT COALESCE(max(update_id),0) FROM telegram_updates').fetchone()[0]
    rows=dst.execute('SELECT id,expires_at FROM pending_actions').fetchall()
    # Restore the synthetic question's validity for this continuation only; record the change.
    expiry=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%SZ')
    dst.execute('UPDATE pending_actions SET expires_at=?',(expiry,));dst.commit()
    resume_info.append({'scenario':c['id'],'source_sha256':before,'start_update_id':start_ids[c['id']],'synthetic_pending_expiry_reset':[{'id':row[0],'from':row[1],'to':expiry} for row in rows]})
   finally:dst.close();src.close()
   if hashlib.sha256(path.read_bytes()).hexdigest()!=before:raise RuntimeError('source recording changed')
 with urllib.request.urlopen('https://openrouter.ai/api/v1/models',timeout=30) as r:metadata=json.load(r)
 entry=next(x for x in metadata['data'] if x['id']==model);(record/'model-metadata.json').write_text(json.dumps(entry,indent=2))
 from PIL import Image,ImageDraw,ImageFont
 image=Image.new('RGB',(700,300),'white');draw=ImageDraw.Draw(image);font=ImageFont.truetype('/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf',30)
 draw.multiline_text((30,25),'Shopping note\n2 cartons oat milk\n1 sourdough loaf\n6 apples',font=font,fill='black',spacing=15);image.save(record/'grocery-note.png')
 relative=record.relative_to(ROOT);prices=[entry['pricing'],*entry['pricing'].get('overrides',[])]
 cfg={'Key':values['OPENROUTER_API_KEY'],'Record':str(relative),'MaxUSD':a.max_usd,'PromptPrice':max(float(x['prompt']) for x in prices),'CompletionPrice':max(float(x['completion']) for x in prices),'Model':model,'Only':a.scenario,'Resume':bool(a.resume),'StartIDs':start_ids}
 effective=[{**c,'steps':c['steps'][1:] if a.resume else c['steps']} for c in selected]
 manifest={'commit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'configuration':{k:v for k,v in cfg.items() if k!='Key'},'resume_source':str(a.resume) if a.resume else None,'resume_details':resume_info,'scenarios':effective}
 (record/'capture-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
 with (record/'runner.log').open('w') as log:
  result=subprocess.run(['toolchain/run','go','run','./cmd/llmprobe'],cwd=ROOT,input=json.dumps(cfg),text=True,stdout=log,stderr=subprocess.STDOUT)
 print('Capture '+('finished' if result.returncode==0 else 'stopped')+'; results: '+str(relative));sys.exit(result.returncode)
except Exception as e:
 print('Capture stopped: '+type(e).__name__+'; no fallback model selected',file=sys.stderr);sys.exit(1)
