#!/usr/bin/env python3
"""Verify recorded proposals and confirmations using actual DB snapshots; no API calls."""
import argparse,json
from decimal import Decimal
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('record',type=Path);p.add_argument('--previous',type=Path,action='append',default=[]);a=p.parse_args()
observations=json.loads((a.record/'observations.json').read_text());checks=[]
def signature(items):return sorted((i['name'],i['quantity'],i.get('unit','')) for i in items)
for scenario in observations['scenarios']:
 proposals=[]
 for step in scenario['steps']:
  for message in step.get('bot_messages',[]):
   if message.get('text','').startswith('Could not finish:'):raise SystemExit('Recorded bot error in '+scenario['scenario'])
  state=step.get('state',{});pending=state.get('question_or_confirmation') or {}
  if pending.get('Kind')=='confirm_add':
   if state.get('pending'):raise SystemExit('Rows inserted before final confirmation')
   proposals.append(pending['Payload'])
 if not proposals:raise SystemExit('No recorded add proposal in '+scenario['scenario'])
 final=scenario['after'];actual=final.get('pending') or []
 if signature(actual)!=signature(proposals[-1]):raise SystemExit('Final rows differ from the actual proposal')
 if final.get('question_or_confirmation'):raise SystemExit('Confirmation not consumed')
 if len({i['id'] for i in actual})!=len(actual):raise SystemExit('Duplicate row IDs')
 checks.append({'scenario':scenario['scenario'],'items_added':len(actual),'zero_insertions_before_confirmation':True,'rows_match_actual_proposal':True,'confirmation_consumed':True})
additional=Decimal(0);total=Decimal(0);unknown=[]
for record in [*a.previous,a.record]:
 for file in sorted(record.glob('request-*.json')):
  entry=json.loads(file.read_text())
  if 'recorded_cost_usd' not in entry:unknown.append(str(file));continue
  cost=Decimal(str(entry['recorded_cost_usd']));total+=cost
  if record==a.record:additional+=cost
result={'checks':checks,'additional_cost_usd':str(additional),'cumulative_cost_usd':str(total),'requests_without_cost':unknown}
(a.record/'checks.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
if unknown:raise SystemExit('Some requests have no recorded cost')
