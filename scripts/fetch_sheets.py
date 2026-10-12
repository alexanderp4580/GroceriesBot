#!/usr/bin/env python3
"""Fetch referenced spreadsheets read-only; save rows privately, print counts only."""
import argparse,json,os,urllib.request,urllib.parse,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
def env_file(path):
 values={}
 for line in path.read_text().splitlines():
  if not line.strip() or line.lstrip().startswith('#'):continue
  key,value=line.split('=',1);values[key.strip()]=json.loads(value)
 return {**values,**os.environ}
def fetch(values):
 payload=urllib.parse.urlencode({'client_id':values['GOOGLE_CLIENT_ID'],'client_secret':values['GOOGLE_CLIENT_SECRET'],'refresh_token':values['GOOGLE_REFRESH_TOKEN'],'grant_type':'refresh_token'}).encode()
 req=urllib.request.Request('https://oauth2.googleapis.com/token',data=payload)
 with urllib.request.urlopen(req,timeout=30) as r:token=json.load(r)['access_token']
 refs=values['GOOGLE_SPREADSHEET_IDS']
 try:ids=json.loads(refs)
 except ValueError:ids=refs.split(',')
 books=[]
 for sid in ids:
  req=urllib.request.Request('https://sheets.googleapis.com/v4/spreadsheets/'+urllib.parse.quote(sid,safe='')+'?includeGridData=true',headers={'Authorization':'Bearer '+token})
  with urllib.request.urlopen(req,timeout=60) as r:books.append(json.load(r))
 return books
if __name__=='__main__':
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('--env',type=Path,default=ROOT/'.env');p.add_argument('--output',type=Path,required=True);a=p.parse_args()
 try:
  books=fetch(env_file(a.env));a.output.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
  with a.output.open('x') as f:json.dump(books,f,ensure_ascii=False)
  print(json.dumps({'spreadsheets':len(books),'sheets':sum(len(b.get('sheets',[])) for b in books)}))
 except Exception as e:print('Sheet fetch failed: '+type(e).__name__,file=sys.stderr);sys.exit(1)
