#!/usr/bin/env python3
"""Run one recorded verification pass. Tests use fixtures; no provider credentials."""
import argparse,datetime,json,os,subprocess,sys
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--record',required=True,type=Path);p.add_argument('--containers',action='store_true');args=p.parse_args()
record=args.record.resolve();record.mkdir(parents=True,exist_ok=False)
steps=[('secret-scan',['python3','scripts/secret_scan.py']),('python-tests',['python3','-m','unittest','discover','-s','tests','-v']),('go-tests',['toolchain/run','go','test','-race','-count=1','./...']),('go-vet',['toolchain/run','go','vet','./...']),('go-build',['toolchain/run','go','build','-trimpath','-o','.local/groceriesbot','./cmd/groceriesbot'])]
if args.containers:steps += [('image-build',['podman','build','-t','localhost/groceriesbot:verify','.']),('container-checks',['python3','scripts/verify_container.py'])]
summary={'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'commit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),'router_requests':'fixture endpoints only; production AI remains disabled','steps':[]}
for name,cmd in steps:
 with (record/(name+'.log')).open('w') as log:
  result=subprocess.run(cmd,cwd=ROOT,stdout=log,stderr=subprocess.STDOUT)
 summary['steps'].append({'name':name,'exit_code':result.returncode,'log':name+'.log'})
 (record/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
 print(name+': '+('PASS' if result.returncode==0 else 'FAIL'),flush=True)
 if result.returncode:sys.exit(result.returncode)
summary['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat();(record/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
