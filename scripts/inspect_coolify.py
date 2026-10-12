import json
import os
import urllib.request
import urllib.error

BASE = 'https://coolify.tail.liveleds.io/api/v1/'
FIELDS = ('uuid', 'name', 'description', 'status', 'type', 'server_id', 'environment_id', 'image', 'database_type', 'version')

def api(path):
    req = urllib.request.Request(BASE + path, headers={
        'Authorization': 'Bearer ' + os.environ['COOLIFY_API_ROOT_TOKEN'],
        'Accept': 'application/json',
    })
    with urllib.request.urlopen(req, timeout=30) as response:
        return json.load(response)

def safe(value):
    if isinstance(value, list):
        return [safe(item) for item in value]
    if isinstance(value, dict):
        result = {key: value[key] for key in FIELDS if key in value}
        for key in ('data', 'services', 'applications', 'databases'):
            if key in value:
                result[key] = safe(value[key])
        return result
    return value

for path in ('version', 'servers', 'services', 'databases', 'projects'):
    try:
        print(json.dumps({'endpoint': path, 'result': safe(api(path))}))
    except urllib.error.HTTPError as error:
        print(json.dumps({'endpoint': path, 'http_status': error.code}))
    except Exception as error:
        print(json.dumps({'endpoint': path, 'error_type': type(error).__name__}))
