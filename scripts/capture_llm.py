#!/usr/bin/env python3
"""Opt-in model recordings from the real processor in an isolated fixture DB."""
import argparse,json,os,subprocess,sys,urllib.request
from pathlib import Path
from fetch_sheets import env_file
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--list',action='store_true');p.add_argument('--record',type=Path);p.add_argument('--allow-paid-model-calls',action='store_true');p.add_argument('--max-usd',type=float,default=.25);a=p.parse_args()
cases=json.loads((ROOT/'testdata/llm-scenarios.json').read_text())
if a.list:
 for c in cases:print(c['id']+': '+c['find_out'])
 sys.exit(0)
if not a.allow_paid_model_calls or not a.record or not 0<a.max_usd<=.25:p.error('capture requires an approval flag, new record path, and budget at most $0.25')
os.umask(0o077);record=a.record.resolve();record.mkdir(parents=True,exist_ok=False)
try:
 values=env_file(ROOT/'.env');model='anthropic/claude-haiku-5.5'
 with urllib.request.urlopen('https://openrouter.ai/api/v1/models',timeout=30) as r:metadata=json.load(r)
 entry=next(x for x in metadata['data'] if x['id']==model)
 (record/'model-metadata.json').write_text(json.dumps(entry,indent=2))
 from PIL import Image,ImageDraw,ImageFont
 image=Image.new('RGB',(700,300),'white');draw=ImageDraw.Draw(image);font=ImageFont.truetype('/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf',30)
 draw.multiline_text((30,25),'Shopping note\n2 cartons oat milk\n1 sourdough loaf\n6 apples',font=font,fill='black',spacing=15);image.save(record/'grocery-note.png')
 relative=record.relative_to(ROOT)
 cfg={'Key':values['OPENROUTER_API_KEY'],'Record':str(relative),'MaxUSD':a.max_usd,'PromptPrice':float(entry['pricing']['prompt']),'CompletionPrice':float(entry['pricing']['completion']),'Model':model}
 with (record/'runner.log').open('w') as log:
  result=subprocess.run(['toolchain/run','go','run','./cmd/llmprobe'],cwd=ROOT,input=json.dumps(cfg),text=True,stdout=log,stderr=subprocess.STDOUT)
 print('Capture '+('finished' if result.returncode==0 else 'stopped')+'; results: '+str(relative));sys.exit(result.returncode)
except Exception as e:
 print('Capture stopped: '+type(e).__name__+'; no fallback model selected',file=sys.stderr);sys.exit(1)
