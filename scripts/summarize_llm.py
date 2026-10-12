#!/usr/bin/env python3
"""Summarize captured facts without inventing missing usage or success verdicts."""
import argparse,json
from decimal import Decimal
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('record',type=Path);a=p.parse_args();requests=[];cost=Decimal(0);unknown=[]
for path in sorted(a.record.glob('request-*.json')):
 v=json.loads(path.read_text());entry={'file':path.name,'scenario':v.get('scenario'),'http_status':v.get('status'),'latency_ms':v.get('latency_ms')}
 if 'recorded_cost_usd' in v:cost+=Decimal(str(v['recorded_cost_usd']));entry['cost_usd']=v['recorded_cost_usd']
 else:unknown.append(path.name)
 response=v.get('response');response=response if isinstance(response,dict) else {}
 usage=response.get('usage');entry['usage']=usage if isinstance(usage,dict) else None
 choices=response.get('choices');entry['choices']=[]
 if isinstance(choices,list):
  for c in choices:
   if not isinstance(c,dict):continue
   m=c.get('message');m=m if isinstance(m,dict) else {}
   calls=m.get('tool_calls');calls=calls if isinstance(calls,list) else []
   entry['choices'].append({'finish_reason':c.get('finish_reason'),'message_fields':sorted(m),'tools':[x.get('function',{}).get('name') for x in calls if isinstance(x,dict) and isinstance(x.get('function',{}),dict)]})
 requests.append(entry)
observations=json.loads((a.record/'observations.json').read_text()) if (a.record/'observations.json').exists() else {}
scenarios=[]
for s in observations.get('scenarios',[]):
 errors=[]
 for step in s.get('steps',[]):
  for message in step.get('bot_messages',[]):
   text=message.get('text','')
   if text.startswith('Could not finish:'):errors.append(text)
  if step.get('note'):errors.append(step['note'])
 scenarios.append({'id':s.get('scenario'),'pending_before':len(s.get('before',{}).get('pending') or []),'pending_after':len(s.get('after',{}).get('pending') or []),'notes':errors})
summary={'recorded_model_requests':len(requests),'recorded_cost_usd':str(cost),'requests_without_cost':unknown,'scenarios':scenarios,'requests':requests}
(a.record/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps({k:v for k,v in summary.items() if k!='requests'},indent=2))
