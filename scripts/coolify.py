#!/usr/bin/env python3
"""Coolify provisioning/status; secrets read privately, responses print metadata only."""
import argparse,json,os,urllib.request,urllib.error,sys
from pathlib import Path
from fetch_sheets import env_file
ROOT=Path(__file__).resolve().parents[1];BASE=os.environ.get('COOLIFY_URL','https://coolify.tail.liveleds.io')+'/api/v1/'
def api(method,path,body=None):
 req=urllib.request.Request(BASE+path,method=method,data=None if body is None else json.dumps(body).encode(),headers={'Authorization':'Bearer '+os.environ['COOLIFY_API_ROOT_TOKEN'],'Content-Type':'application/json','Accept':'application/json'})
 with urllib.request.urlopen(req,timeout=60) as r:return json.load(r)
def metadata(v):
 if isinstance(v,list):return [metadata(x) for x in v]
 if isinstance(v,dict):return {k:metadata(x) for k,x in v.items() if k in ['uuid','name','status','deployment_uuid','deployments','message','git_repository','git_branch','build_pack','docker_compose_location','docker_compose_custom_start_command','autogenerate_domain','docker_compose','persistent_storages','mount_path','host_path','volume_name','value','key'] and k not in ['value','docker_compose']}
 return v
p=argparse.ArgumentParser();p.add_argument('action',choices=['create','status','envs','deploy','stop','activate']);p.add_argument('--uuid');a=p.parse_args()
try:
 if a.action=='create':
  result=api('POST','applications/private-github-app',{'project_uuid':'h8z19ucshyj91wms6smk1seq','server_uuid':'v3odz6rd56ro7lpnh3xoot27','environment_uuid':'lcoazyluc1vljuq8bsl7s3yg','github_app_uuid':'kwp2tihgh7vxnciyoq4s52nf','git_repository':'alexanderp4580/GroceriesBot','git_branch':'main','build_pack':'dockercompose','docker_compose_location':'/deploy/compose.yaml','name':'GroceriesBot','description':'Go grocery bot / SQLite / persistent data / AI credit gate','instant_deploy':False,'is_auto_deploy_enabled':False,'autogenerate_domain':False,'is_preview_deployments_enabled':False,'docker_compose_custom_start_command':'docker compose up -d --remove-orphans','limits_memory':'128m','limits_cpus':'0.5'})
 elif not a.uuid:raise ValueError('application UUID required')
 elif a.action=='status':result=api('GET','applications/'+a.uuid)
 elif a.action=='envs':
  v=env_file(ROOT/'.env');data=[{'key':k,'value':v[k],'is_buildtime':False,'is_runtime':True,'is_literal':True,'is_preview':False} for k in ['TELEGRAM_BOT_TOKEN','OPENROUTER_API_KEY']];data.append({'key':'RUN_MODE','value':'paused','is_buildtime':False,'is_runtime':True,'is_literal':True,'is_preview':False});result=api('PATCH','applications/'+a.uuid+'/envs/bulk',{'data':data})
 elif a.action=='activate':result=api('PATCH','applications/'+a.uuid+'/envs/bulk',{'data':[{'key':'RUN_MODE','value':'active','is_buildtime':False,'is_runtime':True,'is_literal':True,'is_preview':False}]})
 elif a.action=='deploy':result=api('GET','deploy?uuid='+a.uuid+'&force=false')
 elif a.action=='stop':result=api('POST','applications/'+a.uuid+'/stop')
 print(json.dumps(metadata(result)))
except urllib.error.HTTPError as e:
 payload=e.read();(ROOT/'.local').mkdir(exist_ok=True);path=ROOT/'.local/coolify-error.json';path.write_bytes(payload);path.chmod(0o600);print('Coolify request failed: HTTP '+str(e.code)+' (private diagnostic saved)',file=sys.stderr);sys.exit(1)
except Exception as e:print('Coolify operation failed: '+type(e).__name__,file=sys.stderr);sys.exit(1)
