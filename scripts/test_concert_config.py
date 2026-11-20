#!/usr/bin/env python3
"""Verify provider configuration in the real Navidrome UI, without enabling it."""
import json,os,pathlib,re,secrets,shutil,socket,subprocess,tempfile,time,urllib.request
from playwright.sync_api import sync_playwright
ROOT=pathlib.Path(__file__).resolve().parents[1]
(ROOT/'test-runtime').mkdir(exist_ok=True)
base=pathlib.Path(tempfile.mkdtemp(prefix='concert-config-',dir=ROOT/'test-runtime'))
for name in ['music','data/plugins','cache']:(base/name).mkdir(parents=True,exist_ok=True)
shutil.copyfile(ROOT/'dist/navidrome-upcoming-concerts-plugin.ndp',base/'data/plugins/navidrome-upcoming-concerts-plugin.ndp')
navidrome=os.environ.get('NAVIDROME_BIN') or shutil.which('navidrome')
with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
url=f'http://127.0.0.1:{port}'
config=base/'navidrome.toml';config.write_text(f"DataFolder = '{base}/data'\nMusicFolder = '{base}/music'\nCacheFolder = '{base}/cache'\nAddress = '127.0.0.1'\nPort = {port}\nAgents = ''\n[Plugins]\nEnabled = true\n")
token=''
def api(path,method='GET',payload=None):
 headers={'Content-Type':'application/json'}
 if token:headers['x-nd-authorization']='Bearer '+token
 req=urllib.request.Request(url+path,method=method,headers=headers,data=json.dumps(payload).encode() if payload is not None else None)
 with urllib.request.urlopen(req,timeout=10) as r:
  b=r.read();return json.loads(b) if b else None
log=open(base/'server.log','w');server=subprocess.Popen([navidrome,'--configfile',str(config)],stdout=log,stderr=log)
try:
 for _ in range(120):
  try:
   if urllib.request.urlopen(url+'/app/',timeout=2).status==200:break
  except OSError:time.sleep(.25)
 auth=api('/auth/createAdmin','POST',{'username':'config-fixture','password':secrets.token_urlsafe(32)});token=auth['token']
 storage={'token':token,'userId':auth['id'],'username':auth['username'],'name':auth['name'],'role':'admin','subsonic-salt':auth['subsonicSalt'],'subsonic-token':auth['subsonicToken'],'is-authenticated':'true'}
 with sync_playwright() as p:
  browser=p.chromium.launch(headless=True,executable_path=shutil.which('chromium'),args=['--no-sandbox'])
  context=browser.new_context(viewport={'width':1280,'height':1000})
  context.add_init_script('const fixtureAuth='+json.dumps(storage)+';for(const [key,value] of Object.entries(fixtureAuth)){localStorage.setItem(key,value)}')
  page=context.new_page();page.goto(url+'/app/#/plugin/navidrome-upcoming-concerts-plugin/show')
  page.get_by_role('tab',name='Concert providers',exact=True).click()
  assert page.get_by_text('Unable to render configuration form.',exact=False).count()==0
  def field(title):
   label=page.locator('label').filter(has_text=re.compile('^'+re.escape(title)+'$'))
   return page.locator('.MuiFormControl-root').filter(has=label).locator('input,textarea:visible').first
  key=field('Ticketmaster developer API key');assert key.get_attribute('type')=='password'
  key.fill('fixture-ui-key-never-real')
  field('Maximum Ticketmaster requests per UTC day').fill('250')
  field('Ticketmaster artist mappings (JSON)').fill('{"fixture-artist":"band_1"}')
  page.screenshot(path=str(base/'desktop.png'),full_page=True)
  assert field('SeatGeek client ID (API key)').get_attribute('type')=='password'
  assert field('SeatGeek client secret (optional)').get_attribute('type')=='password'
  field('SeatGeek client ID (API key)').fill('fixture-sg-client')
  field('SeatGeek client secret (optional)').fill('fixture-sg-secret')
  field('SeatGeek local daily request budget').fill('200')
  field('SeatGeek artist mappings (JSON)').fill('{"fixture-artist":"123"}')
  assert field('Skiddle API key').get_attribute('type')=='password'
  field('Skiddle API key').fill('fixture-sk-key')
  field('Skiddle local daily request budget').fill('150')
  field('Skiddle artist mappings (JSON)').fill('{"fixture-artist":"456"}')
  page.get_by_role('checkbox',name='Automatically match a single exact-name artist',exact=True).uncheck()
  page.get_by_role('checkbox',name='Deduplicate concerts across sources',exact=True).uncheck()
  page.get_by_role('button',name='Save',exact=True).click()
  for _ in range(80):
   saved=api('/api/plugin/navidrome-upcoming-concerts-plugin')
   values=json.loads(saved.get('config') or '{}')
   if values.get('ticketmaster_daily_budget')==250:break
   time.sleep(.25)
  assert values['provider_auto_match']==False
  assert values['skiddle_api_key']=='fixture-sk-key' and values['skiddle_daily_budget']==150
  assert values['seatgeek_client_id']=='fixture-sg-client' and values['seatgeek_client_secret']=='fixture-sg-secret'
  assert values['seatgeek_daily_budget']==200 and values['deduplicate_concerts']==False
  assert values['ticketmaster_api_key']=='fixture-ui-key-never-real'
  assert json.loads(values['ticketmaster_artist_overrides'])=={'fixture-artist':'band_1'}
  assert not saved['enabled'],'configuration test enabled provider unexpectedly'
  page.reload();page.get_by_role('tab',name='Concert providers',exact=True).click()
  assert field('Ticketmaster developer API key').input_value()=='fixture-ui-key-never-real'
  assert field('Maximum Ticketmaster requests per UTC day').input_value()=='250'
  assert field('SeatGeek client ID (API key)').input_value()=='fixture-sg-client'
  assert not page.get_by_role('checkbox',name='Automatically match a single exact-name artist',exact=True).is_checked()
  assert not page.get_by_role('checkbox',name='Deduplicate concerts across sources',exact=True).is_checked()
  page.set_viewport_size({'width':390,'height':900});page.screenshot(path=str(base/'mobile.png'),full_page=True)
  browser.close()
 print('Navidrome provider configuration passed: rendering, masked key, budget, mappings, save and reload; plugin remains disabled.')
 print('Disposable artifacts:',base)
finally:
 server.terminate();server.wait(timeout=15);log.close()
