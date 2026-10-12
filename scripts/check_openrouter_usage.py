#!/usr/bin/env python3
"""Read account/key usage without model requests; save raw responses privately."""
import argparse,json,os,urllib.request,urllib.error
from pathlib import Path
from fetch_sheets import env_file
ROOT=Path(__file__).resolve().parents[1];p=argparse.ArgumentParser();p.add_argument('--record',type=Path,required=True);a=p.parse_args();os.umask(0o077);a.record.mkdir(parents=True,exist_ok=False);v=env_file(ROOT/'.env')
for route in ['key','credits']:
 req=urllib.request.Request('https://openrouter.ai/api/v1/'+route,headers={'Authorization':'Bearer '+v['OPENROUTER_API_KEY']})
 try:
  with urllib.request.urlopen(req,timeout=10) as r:data=json.load(r)
  (a.record/(route+'.json')).write_text(json.dumps(data,indent=2));d=data.get('data',{});safe={k:d.get(k) for k in ['usage','usage_daily','usage_monthly','limit','limit_remaining','total_credits','total_usage','is_management_key'] if k in d};print(json.dumps({'endpoint':route,'status':200,'values':safe}))
 except urllib.error.HTTPError as e:(a.record/(route+'-error.json')).write_bytes(e.read());print(json.dumps({'endpoint':route,'status':e.code}))
