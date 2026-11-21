#!/usr/bin/env python3
"""Actual Navidrome 0.64.2 / WASM / HTTP-host test with disposable fixture data.
Requires NAVIDROME_BIN, Go 1.25, TinyGo 0.40.1 and ffmpeg. No provider account.
"""
import datetime,http.server,json,os,pathlib,secrets,shutil,socket,subprocess,tempfile,threading,time,urllib.request,zipfile
from urllib.parse import urlsplit,parse_qs
ROOT=pathlib.Path(__file__).resolve().parents[1]
(ROOT/'test-runtime').mkdir(exist_ok=True)
base=pathlib.Path(tempfile.mkdtemp(prefix='concert-',dir=ROOT/'test-runtime'))
for name in ['music','data/plugins','cache']:(base/name).mkdir(parents=True,exist_ok=True)
navidrome=os.environ.get('NAVIDROME_BIN') or shutil.which('navidrome')
if not navidrome:raise SystemExit('Set NAVIDROME_BIN to Navidrome 0.64.2')
mbid='a74b1b7f-71a5-4011-9441-d0b5e4122711'
subprocess.run(['ffmpeg','-hide_banner','-loglevel','error','-f','lavfi','-i','sine=frequency=440:duration=1','-metadata','artist=Fixture Band','-metadata','album_artist=Fixture Band','-metadata','album=Fixture','-metadata','title=Test tone','-metadata','MUSICBRAINZ_ARTISTID='+mbid,'-metadata','MUSICBRAINZ_ALBUMARTISTID='+mbid,str(base/'music/test.flac')],check=True)
storage=base/'data/plugins/navidrome-upcoming-concerts-plugin/storage'
requests=[];sg_requests=[];sk_requests=[];failures=[]
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  try:
   parsed=urlsplit(self.path);q=parse_qs(parsed.query)
   if parsed.path.startswith('/tm/'):
    assert q['apikey']==['fixture-key-never-real']
    requests.append(self.path)
    saved=json.loads((storage/'ticketmaster.json').read_text())
    assert saved['requestsReserved']>=len(requests),'HTTP preceded durable request reservation'
    if parsed.path.endswith('attractions.json'):
     body={'page':{'totalElements':1},'_embedded':{'attractions':[{'id':'band_1','name':'Fixture Band','externalLinks':{'musicbrainz':[{'url':'https://musicbrainz.org/artist/'+mbid}]}}]}}
    else:
     page=int(q['page'][0]);assert q['attractionId']==['band_1']
     date=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=5+page)).strftime('%Y-%m-%d')
     body={'page':{'number':page,'totalElements':2,'totalPages':2},'_embedded':{'events':[{'id':'event_'+str(page),'name':'Fixture concert <script>unsafe</script>','url':'https://www.ticketmaster.com/event/fixture'+str(page),'dates':{'start':{'localDate':date},'status':{'code':'onsale' if page==0 else 'canceled'}},'_embedded':{'attractions':[{'id':'band_1'}],'venues':[{'name':'Fixture Hall','city':{'name':'London'},'country':{'countryCode':'GB'}}]}}]}}
   elif parsed.path.startswith('/sg/'):
    assert q['client_id']==['fixture-sg-client'] and q['client_secret']==['fixture-sg-secret']
    sg_requests.append(self.path)
    saved=json.loads((storage/'seatgeek.json').read_text())
    assert saved['requestsReserved']>=len(sg_requests)
    if parsed.path.endswith('/performers'):
     body={'meta':{'page':1,'per_page':200,'total':1},'performers':[{'id':123,'name':'Fixture Band'}]}
    else:
     page=int(q['page'][0]);assert q['performers.id']==['123']
     day=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=4+page)).strftime('%Y-%m-%d')
     body={'meta':{'page':page,'per_page':1,'total':2},'events':[{'id':page,'title':'Fixture concert <script>unsafe</script>','url':'https://seatgeek.com/fixture/'+str(page),'type':'concert','datetime_local':day+'T03:30:00','time_tbd':True,'status':'onsale' if page==1 else 'cancelled','performers':[{'id':123}],'venue':{'name':'Fixture Hall','city':'London','country':'GB'}}]}
   elif parsed.path.startswith('/sk/'):
    assert q['api_key']==['fixture-sk-key']
    assert 'navidrome-upcoming-concerts-plugin/' in self.headers.get('User-Agent','')
    sk_requests.append(self.path);assert json.loads((storage/'skiddle.json').read_text())['requestsReserved']>=len(sk_requests)
    if parsed.path.endswith('/artists/'):
     body={'error':0,'totalcount':1,'results':[{'id':'456','name':'Fixture Band'}]}
    else:
     assert q['a']==['456'] and q['description']==['1']
     events=[]
     for offset in [0,1]:
      day=(datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=5+offset)).strftime('%Y-%m-%d')
      events.append({'id':str(789+offset),'eventname':'Fixture concert <script>unsafe</script>','date':day,'link':'https://www.skiddle.com/whats-on/fixture/'+str(789+offset)+'/','cancelled':str(offset),'artists':[{'artistid':'456'}],'venue':{'name':'Fixture Hall','town':'London','countrycode':'GB'},'openingtimes':{'doorsopen':'19:00'}})
     body={'error':0,'totalcount':'2','results':events}
   else:body={'id':mbid,'relations':[]}
   data=json.dumps(body).encode();self.send_response(200);self.end_headers();self.wfile.write(data)
  except Exception as e:
   failures.append(repr(e));self.send_response(500);self.end_headers()
 def log_message(self,*args):pass
fixture=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler)
threading.Thread(target=fixture.serve_forever,daemon=True).start()
fixture_url=f'http://127.0.0.1:{fixture.server_port}'
source=ROOT/'internal/radar/concert_provider_fixture.go'
source.write_text('//go:build concert_fixture\n\npackage radar\nfunc init(){mbBase='+json.dumps(fixture_url+'/mb/')+';ticketmasterBase='+json.dumps(fixture_url+'/tm/')+';seatGeekBase='+json.dumps(fixture_url+'/sg/')+';skiddleBase='+json.dumps(fixture_url+'/sk/')+'}\n')
try:
 subprocess.run(['tinygo','build','-no-debug','-panic=trap','-o',str(base/'fixture.wasm'),'-target','wasip1','-buildmode=c-shared','-tags','concert_fixture','./cmd/plugin'],cwd=ROOT,check=True)
finally:source.unlink(missing_ok=True)
m=json.loads((ROOT/'manifest.json').read_text());m['permissions']['http']['requiredHosts']=['127.0.0.1']
with zipfile.ZipFile(base/'data/plugins/navidrome-upcoming-concerts-plugin.ndp','w',zipfile.ZIP_DEFLATED) as z:
 z.writestr('manifest.json',json.dumps(m));z.write(base/'fixture.wasm','plugin.wasm')
with socket.socket() as available:available.bind(('127.0.0.1',0));port=available.getsockname()[1]
config=base/'navidrome.toml';config.write_text(f"DataFolder = '{base}/data'\nMusicFolder = '{base}/music'\nCacheFolder = '{base}/cache'\nAddress = '127.0.0.1'\nPort = {port}\nLogLevel = 'debug'\nAgents = ''\n[Plugins]\nEnabled = true\n")
url=f'http://127.0.0.1:{port}';token='';server=None;log=None

def api(path,method='GET',payload=None):
 headers={'Content-Type':'application/json'}
 if token:headers['x-nd-authorization']='Bearer '+token
 req=urllib.request.Request(url+path,method=method,headers=headers,data=json.dumps(payload).encode() if payload is not None else None)
 with urllib.request.urlopen(req,timeout=10) as response:
  body=response.read();return json.loads(body) if body else None

def wait_for(predicate,timeout=60):
 end=time.monotonic()+timeout
 while time.monotonic()<end:
  if failures:raise AssertionError(failures)
  if (base/'server.log').exists() and 'Plugin init function failed' in (base/'server.log').read_text():raise AssertionError('WASM initialization failed; inspect '+str(base/'server.log'))
  try:
   result=predicate()
   if result:return result
  except (OSError,KeyError,ValueError,IndexError):pass
  time.sleep(.25)
 raise AssertionError('Timed out; inspect '+str(base/'server.log'))

def start():
 global server,log
 log=open(base/'server.log','a');server=subprocess.Popen([navidrome,'--configfile',str(config)],stdout=log,stderr=log)
 wait_for(lambda:urllib.request.urlopen(url+'/app/',timeout=2).status==200)
def stop():
 global server
 if server:server.terminate();server.wait(timeout=15);log.close();server=None

def provider():return json.loads((storage/'ticketmaster.json').read_text())
settings={'ticketmaster_enabled':True,'ticketmaster_api_key':'fixture-key-never-real','ticketmaster_daily_budget':3}
try:
 start();auth=api('/auth/createAdmin','POST',{'username':'fixture-admin','password':secrets.token_urlsafe(32)});token=auth['token']
 def artists():
  from urllib.parse import urlencode
  q=urlencode({'u':'fixture-admin','t':auth['subsonicToken'],'s':auth['subsonicSalt'],'v':'1.16.1','c':'concert-test','f':'json'})
  r=api('/rest/getArtists?'+q)['subsonic-response']
  return [a for group in r.get('artists',{}).get('index',[]) for a in group.get('artist',[])]
 discovered=wait_for(artists)
 settings.update({'skiddle_enabled':True,'skiddle_api_key':'fixture-sk-key','skiddle_daily_budget':2})
 settings.update({'seatgeek_enabled':True,'seatgeek_client_id':'fixture-sg-client','seatgeek_client_secret':'fixture-sg-secret','seatgeek_daily_budget':3})
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'enabled':True,'allUsers':True,'allLibraries':True,'allowWriteAccess':False,'config':json.dumps(settings)})
 wait_for(lambda:provider()['artists'][0]['lookup']['outcome']=='found',120)
 wait_for(lambda:json.loads((storage/'seatgeek.json').read_text())['artists'][0]['lookup']['outcome']=='found')
 wait_for(lambda:json.loads((storage/'skiddle.json').read_text())['artists'][0]['lookup']['outcome']=='found')
 state=provider();assert state['requestsReserved']==3 and len(requests)==3
 assert len(state['artists'][0]['concerts'])==2
 wait_for(lambda:(storage/'upcoming-concerts.html').read_text().count('<article class="concert"')==2 and 'Source: SeatGeek' in (storage/'upcoming-concerts.html').read_text() and 'Source: Ticketmaster' in (storage/'upcoming-concerts.html').read_text() and 'Source: Skiddle' in (storage/'upcoming-concerts.html').read_text())
 html=(storage/'upcoming-concerts.html').read_text()
 assert 'Fixture Hall, London, GB' in html and 'Source: Ticketmaster' in html and 'Source: SeatGeek' in html
 assert html.count('<article class="concert"')==2
 assert 'Source: Skiddle' in html and 'Skiddle logo' in html
 assert len(sk_requests)==2
 assert len(sg_requests)==3
 assert '&lt;script&gt;unsafe&lt;/script&gt;' in html and '<script>unsafe</script>' not in html
 ics=(storage/'music-concerts.ics').read_text();assert ics.count('BEGIN:VEVENT')==2 and 'STATUS:CANCELLED' in ics
 assert (storage/'upcoming-concerts.html').stat().st_mode & 0o777 == 0o600
 assert 'fixture-key-never-real' not in (storage/'ticketmaster.json').read_text()
 stop()
 # Simulate a committed provider snapshot whose HTML publication was interrupted.
 cached=json.loads((storage/'seatgeek.json').read_text());cached['revision']=cached.get('revision',0)+1
 cached['artists'][0]['lookup']['lastSuccessAt']=int(time.time())+1
 cached['artists'][0]['concerts'][0]['title']='Restart-updated fixture concert'
 (storage/'seatgeek.json').write_text(json.dumps(cached))
 (storage/'tick.lock').write_text('simulated interrupted callback');start()
 wait_for(lambda:'Restart-updated fixture concert' in (storage/'upcoming-concerts.html').read_text())
 # Establish a completed credit inventory, then ensure a manual rescan
 # publishes the authorized provider caches and reaches every provider queue.
 settings['exclude_uncredited']=True
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'config':json.dumps(settings)})
 def local_state():return json.loads((storage/'state.json').read_text())
 wait_for(lambda:local_state().get('directCredits',{}).get('ready') and 'Checking directly credited music:' not in local_state().get('problem',''))
 settings['manual_refresh']=True
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'config':json.dumps(settings)})
 def cached_rescan():
  doc=(storage/'upcoming-concerts.html').read_text()
  return 'Checking directly credited music:' in doc and 'Source: SeatGeek' in doc and 'Source: Skiddle' in doc
 wait_for(cached_rescan)
 assert (storage/'music-concerts.ics').read_text().count('BEGIN:VEVENT')==2
 wait_for(lambda:local_state().get('refreshToggle') and not local_state().get('providerRefreshPending') and 'Checking directly credited music:' not in local_state().get('problem',''))
 assert len(requests)==3 and provider()['requestsReserved']==3,'restart or refresh reset quota'
 for name in ['ticketmaster','seatgeek','skiddle']:
  snapshot=json.loads((storage/(name+'.json')).read_text())
  assert snapshot['artists'][0]['lookup']['nextRefreshAt']==0,'credit scan swallowed manual provider refresh'
  assert len(snapshot['artists'][0]['concerts'])==2,'refresh lost cached concerts'
 settings['deduplicate_concerts']=False
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'config':json.dumps(settings)})
 wait_for(lambda:(storage/'music-concerts.ics').read_text().count('BEGIN:VEVENT')==6)
 settings['deduplicate_concerts']=True
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'config':json.dumps(settings)})
 wait_for(lambda:(storage/'music-concerts.ics').read_text().count('BEGIN:VEVENT')==2)
 assert len(sk_requests)==2 and len(sg_requests)==3 and len(requests)==3,'deduplication switch re-fetched metadata'
 # A damaged provider cache is reported without dropping healthy sources.
 skiddle_file=storage/'skiddle.json'
 skiddle_cache=skiddle_file.read_bytes()
 skiddle_file.write_text('{')
 wait_for(lambda:'skiddle.json is corrupt' in (storage/'upcoming-concerts.html').read_text())
 assert 'Source: SeatGeek' in (storage/'upcoming-concerts.html').read_text()
 assert (storage/'music-concerts.ics').read_text().count('BEGIN:VEVENT')==2
 assert skiddle_file.read_text()=='{','damaged provider cache was overwritten'
 skiddle_file.write_bytes(skiddle_cache)
 wait_for(lambda:'Source: Skiddle' in (storage/'upcoming-concerts.html').read_text())
 # Malformed settings must preserve the last valid scan and report.
 before_state=(storage/'state.json').read_bytes();before_html=(storage/'upcoming-concerts.html').read_bytes()
 invalid=dict(settings,seatgeek_artist_overrides='[')
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'config':json.dumps(invalid)})
 time.sleep(3)
 assert (storage/'state.json').read_bytes()==before_state,'invalid settings erased or changed state'
 assert (storage/'upcoming-concerts.html').read_bytes()==before_html,'invalid settings replaced good report'
 api('/api/plugin/navidrome-upcoming-concerts-plugin','PUT',{'config':json.dumps(settings)})
 assert not (base/'music/upcoming-concerts.html').exists()
 print('Navidrome 0.64.2 integration passed: library discovery, host HTTP, storage, automatic exact-name identity, pagination, invalid-setting preservation, concert output, cancellation, escaping, restart, independent provider quotas, refresh and cross-source deduplication on/off.')
 print('Disposable test artifacts:',base)
finally:
 stop();fixture.shutdown()
