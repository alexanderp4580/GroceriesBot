#!/usr/bin/env python3
"""Measure Linux container RSS and cumulative CPU without environment or logs."""
import argparse,json,subprocess,time,datetime
p=argparse.ArgumentParser();p.add_argument('container');p.add_argument('--engine',choices=['docker','podman'],default='docker');p.add_argument('--seconds',type=int,default=600);p.add_argument('--interval',type=int,default=10);p.add_argument('--output',required=True);a=p.parse_args()
info=json.loads(subprocess.check_output([a.engine,'inspect',a.container],text=True))[0];pid=info['State']['Pid'];children=open('/proc/'+str(pid)+'/task/'+str(pid)+'/children').read().split();pid=next((int(c) for c in children if 'groceriesbot' in open('/proc/'+c+'/comm').read()),pid);samples=[];start=time.monotonic()
def sample():
 status={l.split(':',1)[0]:l.split(':',1)[1].strip() for l in open('/proc/'+str(pid)+'/status') if ':' in l}
 import os
 parts=open('/proc/'+str(pid)+'/stat').read().split(') ',1)[1].split();cpu=(int(parts[11])+int(parts[12]))/os.sysconf('SC_CLK_TCK')
 return {'elapsed_seconds':round(time.monotonic()-start,3),'rss_kib':int(status['VmRSS'].split()[0]),'cpu_seconds':cpu}
samples.append(sample())
while time.monotonic()-start<a.seconds:
 time.sleep(min(a.interval,max(.01,a.seconds-(time.monotonic()-start))));samples.append(sample())
duration=samples[-1]['elapsed_seconds']-samples[0]['elapsed_seconds'];result={'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'container':a.container,'duration_seconds':duration,'max_rss_mib':max(s['rss_kib'] for s in samples)/1024,'cpu_percent_one_core':100*(samples[-1]['cpu_seconds']-samples[0]['cpu_seconds'])/duration,'samples':samples}
with open(a.output,'x') as f:json.dump(result,f,indent=2)
print(json.dumps({k:v for k,v in result.items() if k!='samples'}))
