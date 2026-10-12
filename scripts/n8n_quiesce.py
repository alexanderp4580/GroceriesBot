#!/usr/bin/env python3
"""Run on the server: disable grocery intake, stop main, require a drained worker."""
import json,shlex,subprocess,time
SERVICE='fwcmxt6ry1vt7p5scuhzwizj'
def sql(query):
 cmd='psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c '+shlex.quote(query)
 return subprocess.check_output(['docker','exec','postgresql-'+SERVICE,'sh','-c',cmd],text=True).strip()
state=sql('SELECT json_build_object(\'id\',id,\'active\',active,\'version\',"activeVersionId") FROM workflow_entity WHERE id=\'SLffBV8YkETQcXcF\'')
print('Previous workflow: '+state,flush=True)
subprocess.run(['docker','exec','-u','node','n8n-'+SERVICE,'n8n','unpublish:workflow','--id=SLffBV8YkETQcXcF'],check=True,stdout=subprocess.DEVNULL)
subprocess.run(['docker','stop','--time','30','n8n-'+SERVICE],check=True,stdout=subprocess.DEVNULL)
deadline=time.monotonic()+45
while True:
 pending=int(sql('SELECT count(*) FROM execution_entity WHERE "workflowId"=\'SLffBV8YkETQcXcF\' AND status IN (\'new\',\'running\',\'waiting\')'))
 if pending==0:break
 if time.monotonic()>deadline:raise RuntimeError('grocery executions remain; do not stop worker or activate replacement')
 time.sleep(2)
print(json.dumps({'intake_stopped':True,'workflow_unpublished':True,'pending_executions':0}))
