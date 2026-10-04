#!/usr/bin/env python3
"""Exercise the local web/auth/API/DB/MCP boundary. No external providers or sends."""
import base64
import hashlib
import http.cookiejar
import json
import os
import secrets
import time
import urllib.error
import urllib.parse
import urllib.request

base = os.environ.get('TULLIPS_TEST_URL', 'http://localhost:3000')
for attempt in range(30):
    try:
        with urllib.request.urlopen(base + '/', timeout=2) as response:
            if response.status == 200: break
    except (OSError, urllib.error.URLError):
        pass
    time.sleep(1)
else:
    raise SystemExit('Web service did not become ready within 30 attempts')
jar = http.cookiejar.CookieJar()
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
def request(path, method='GET', data=None, token=None, form=False, expected=200):
    headers = {'Origin': base}
    if token: headers['Authorization'] = 'Bearer ' + token
    payload = None
    if data is not None:
        headers['Content-Type'] = 'application/x-www-form-urlencoded' if form else 'application/json'
        payload = (urllib.parse.urlencode(data) if form else json.dumps(data)).encode()
    req = urllib.request.Request(base + path, data=payload, headers=headers, method=method)
    try: response = client.open(req)
    except urllib.error.HTTPError as error: response = error
    body = response.read().decode()
    assert response.status == expected, (path, response.status, body[:500])
    return json.loads(body) if body else None

user = request('/api/auth/sign-up/email', 'POST', {'name':'Offline test owner','email':f'test-{secrets.token_hex(6)}@example.test','password':secrets.token_urlsafe(24)})
workspace = request('/api/v1/workspaces','POST',{'name':'Offline acceptance test'})
ws = workspace['id']
prefix = '/api/v1/workspaces/' + ws
try:
    prospect = request(prefix+'/prospects','POST',{'first_name':'Test','last_name':'Prospect','company':'Example','linkedin_url':'https://www.linkedin.com/in/offline-test','tags':['test']})
    request(prefix+'/prospects/'+prospect['id'],'PATCH',{'notes':'Saved through the authenticated proxy'})
    found = request(prefix+'/prospects')
    assert any(p['id']==prospect['id'] and p['notes'].startswith('Saved') for p in found)
    key = request(prefix+'/keys','POST',{'name':'read-only smoke','scopes':['read']})
    mcp = request('/mcp','POST',{'jsonrpc':'2.0','id':1,'method':'tools/list'},key['token'])
    assert all(t['name'].startswith(('list_','get_')) for t in mcp['result']['tools'])
    denied = request('/mcp','POST',{'jsonrpc':'2.0','id':2,'method':'tools/call','params':{'name':'create_prospect','arguments':{'workspace_id':ws,'data':{'first_name':'Denied'}}}},key['token'])
    assert 'error' in denied
    reg = request('/oauth/register','POST',{'client_name':'Offline MCP test','redirect_uris':['http://127.0.0.1:9234/callback']},expected=201)
    verifier = secrets.token_urlsafe(48)
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).decode().rstrip('=')
    resource = base + '/mcp'
    grant = request('/api/v1/oauth/authorize','POST',{'client_id':reg['client_id'],'redirect_uri':'http://127.0.0.1:9234/callback','code_challenge':challenge,'code_challenge_method':'S256','resource':resource,'scope':'read','state':'test-state','workspace_id':ws})
    code = urllib.parse.parse_qs(urllib.parse.urlsplit(grant['redirect_uri']).query)['code'][0]
    token_data = {'grant_type':'authorization_code','client_id':reg['client_id'],'redirect_uri':'http://127.0.0.1:9234/callback','code_verifier':verifier,'resource':resource,'code':code}
    token = request('/oauth/token','POST',token_data,form=True)
    request('/oauth/token','POST',token_data,form=True,expected=400)
    result = request('/mcp','POST',{'jsonrpc':'2.0','id':3,'method':'tools/call','params':{'name':'list_prospects','arguments':{'workspace_id':ws}}},token['access_token'])
    assert not result['result']['isError']
    request('/oauth/revoke','POST',{'token':token['access_token'],'client_id':reg['client_id']},form=True)
    request('/mcp','POST',{'jsonrpc':'2.0','id':4,'method':'tools/list'},token['access_token'],expected=401)
    request(prefix+'/keys/'+key['id'],'DELETE')
    request('/mcp','POST',{'jsonrpc':'2.0','id':5,'method':'tools/list'},key['token'],expected=401)
    print('PASS: signup, workspace, persistent CRM edits, API-key scopes/revocation, OAuth PKCE/exchange/replay/revocation, MCP tools')
finally:
    request(prefix,'DELETE')
