const $=id=>document.getElementById(id);let snapshot=null,editingId='',currentSettings={enginePath:'',theme:'green'},settingsLoaded=false;
const API={
 async request(path,method='GET',body){const options={method,headers:{'Accept':'application/json'}};if(body!==undefined){options.headers['Content-Type']='application/json';options.body=JSON.stringify(body)}const response=await fetch('/api/'+path,options);let payload={};try{payload=await response.json()}catch{}if(!response.ok)throw new Error(payload.error||('HTTP '+response.status));return payload.data},
 GetSnapshot:()=>API.request('snapshot'),
 GetLogs:()=>API.request('logs'),
 GetProfiles:()=>API.request('profiles'),
 GetSettings:()=>API.request('settings'),
 StartTunnel:()=>API.request('tunnel/start','POST',{}),
 StopTunnel:()=>API.request('tunnel/stop','POST',{}),
 RestartTunnel:()=>API.request('tunnel/restart','POST',{}),
 StartBackground:()=>API.request('background/start','POST',{}),
 StopBackground:()=>API.request('background/stop','POST',{}),
 SaveProfile:p=>API.request('profiles/save','POST',{profile:p}),
 SetActiveProfile:id=>API.request('profiles/active','POST',{id}),
 DeleteProfile:id=>API.request('profiles/delete','POST',{id}),
 SaveSettings:settings=>API.request('settings/save','POST',{settings}),
 UninstallApp:purgeData=>API.request('uninstall','POST',{purgeData}),
 InstallOrUpdateEngine:()=>API.request('engine/install','POST',{}),
 RunDiagnostics:()=>API.request('diagnostics','POST',{}),
 ClearLogs:()=>API.request('logs/clear','POST',{}),
 Quit:()=>API.request('shutdown','POST',{})
};
function applyTheme(theme){const allowed=['green','cyan','blue','purple','orange','rose'];const selected=allowed.includes(theme)?theme:'green';document.documentElement.dataset.theme=selected;if($('theme-select'))$('theme-select').value=selected}
function toast(s){const t=$('toast');t.textContent=s;t.style.display='block';setTimeout(()=>t.style.display='none',4500)}
async function call(name,...args){try{const fn=API[name];if(typeof fn!=='function')throw new Error('Unknown API action: '+name);const r=await fn(...args);await refresh();return r}catch(e){toast(e?.message||String(e));throw e}}
function safe(fn){return (...args)=>Promise.resolve(fn(...args)).catch(()=>{})}
function page(id){document.querySelectorAll('.nav').forEach(n=>n.classList.toggle('active',n.dataset.page===id));document.querySelectorAll('.section').forEach(n=>n.classList.toggle('active',n.id===id));$('title').textContent=id[0].toUpperCase()+id.slice(1)}
document.querySelectorAll('.nav').forEach(n=>n.addEventListener('click',()=>page(n.dataset.page)));
let refreshing=false,localUIStopped=false;
async function refresh(){
 if(refreshing||localUIStopped)return;
 refreshing=true;
 try{
  snapshot=await API.GetSnapshot();
  if(!settingsLoaded){currentSettings=await API.GetSettings();currentSettings.theme=currentSettings.theme||'green';applyTheme(currentSettings.theme);settingsLoaded=true}
  $('version').textContent='Desktop '+snapshot.appVersion;
  $('state-pill').textContent=snapshot.connectionState;
  $('state').textContent=snapshot.connectionState;
  $('substate').textContent=snapshot.lastError||snapshot.mode;
  $('engine').textContent=snapshot.engineFound?snapshot.engineVersion:'Not installed';
  $('active-profile').textContent=snapshot.activeProfile.name;
  $('mode').textContent=snapshot.mode;
  $('uptime').textContent=snapshot.uptime;
  $('target').textContent=snapshot.activeProfile.localTCP;
  $('region').textContent=snapshot.activeProfile.region;
  $('udp').textContent=snapshot.activeProfile.localUDPPort;
  $('engine-path').textContent=snapshot.enginePath||'Not installed';
  $('bg-support').textContent=snapshot.backgroundSupported?'Available':'Unavailable';
  $('start').disabled=snapshot.running||snapshot.backgroundRunning;
  $('stop').disabled=!snapshot.running&&!snapshot.backgroundRunning;
  $('restart').disabled=!snapshot.running&&!snapshot.backgroundRunning;
  $('background-start').disabled=snapshot.running||snapshot.backgroundRunning||!snapshot.backgroundSupported;
  $('background-stop').disabled=!snapshot.backgroundRunning;
  renderProfiles(snapshot.profiles);
  renderLogs(await API.GetLogs());
 }catch(e){
  $('state').textContent='Backend unavailable';
  $('substate').textContent=e.message||String(e);
  $('state-pill').textContent='Connection error';
 }finally{
  refreshing=false;
 }
}
function renderProfiles(items){$('profile-list').classList.toggle('empty',!items.length);$('profile-list').innerHTML=items.map(p=>'<div class="entry"><b>'+esc(p.name)+'</b><br><small>'+esc(p.region)+' · '+esc(p.localTCP)+' · UDP '+p.localUDPPort+'</small><div class="profile-actions" style="margin-top:9px"><button class="btn" data-use="'+esc(p.id)+'">Use</button><button class="btn" data-edit="'+esc(p.id)+'">Edit</button><button class="btn danger" data-delete="'+esc(p.id)+'">Delete</button></div></div>').join('')||'No profiles saved';document.querySelectorAll('[data-use]').forEach(b=>b.onclick=safe(async()=>{await call('SetActiveProfile',b.dataset.use)}));document.querySelectorAll('[data-edit]').forEach(b=>b.onclick=()=>{const p=items.find(x=>x.id===b.dataset.edit);if(!p)return;editingId=p.id;$('p-name').value=p.name;$('p-region').value=p.region;$('p-tcp').value=p.localTCP;$('p-ip').value=p.localUDPIP;$('p-udp').value=p.localUDPPort});document.querySelectorAll('[data-delete]').forEach(b=>b.onclick=safe(async()=>{if(confirm('Delete this profile?'))await call('DeleteProfile',b.dataset.delete)}))}
function renderLogs(items){$('logs-list').innerHTML=items.length?items.slice().reverse().map(l=>'<div class="log"><span class="'+(l.level==='ERROR'?'fail':l.level==='WARN'?'warn':'ok')+'">'+esc(l.time)+' ['+esc(l.level)+']</span> '+esc(l.message)+'</div>').join(''):'<div class="empty">No logs yet.</div>';$('recent-logs').innerHTML=items.length?items.slice(-4).reverse().map(l=>'<div class="row"><span>'+esc(l.time)+' '+esc(l.message)+'</span><b>'+esc(l.level)+'</b></div>').join(''):'No tunnel output yet.'}
function esc(v){return String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
$('start').onclick=safe(()=>call('StartTunnel'));$('stop').onclick=safe(()=>call('StopTunnel'));$('restart').onclick=safe(()=>call('RestartTunnel'));$('background-start').onclick=safe(()=>call('StartBackground'));$('background-stop').onclick=safe(()=>call('StopBackground'));
$('new-profile').onclick=()=>{$('p-name').value='';$('p-region').value='AUTO';$('p-tcp').value='127.0.0.1:19132';$('p-ip').value='';$('p-udp').value='47334';editingId=''};
$('reset-profile').onclick=$('new-profile').onclick;
$('save-profile').onclick=safe(async()=>{const p={id:editingId,name:$('p-name').value,region:$('p-region').value,localTCP:$('p-tcp').value,localUDPIP:$('p-ip').value,localUDPPort:Number($('p-udp').value)};await call('SaveProfile',p);toast('Profile saved')});
async function diagnostics(){const out=await call('RunDiagnostics');$('diag-results').innerHTML=out.map(x=>'<div class="entry"><b class="'+esc(x.status)+'">'+esc(x.status.toUpperCase())+' · '+esc(x.name)+'</b><p>'+esc(x.details)+'</p></div>').join('')}
$('diag').onclick=safe(diagnostics);$('run-diag').onclick=safe(()=>{page('diagnostics');return diagnostics()});$('refresh-logs').onclick=safe(async()=>renderLogs(await API.GetLogs()));$('clear-logs').onclick=safe(()=>call('ClearLogs'));
$('install-engine').onclick=safe(async()=>{const b=$('install-engine');b.disabled=true;b.textContent='Installing…';try{const x=await call('InstallOrUpdateEngine');toast('Installed '+x.version)}finally{b.disabled=false;b.textContent='Install / update engine'}});
$('choose-engine').onclick=safe(async()=>{currentSettings=await API.SaveSettings({...currentSettings,enginePath:$('custom-engine').value.trim()});await refresh();toast('Engine path saved')});
$('theme-select').onchange=safe(async()=>{const theme=$('theme-select').value;currentSettings=await API.SaveSettings({...currentSettings,theme});applyTheme(theme);toast('Theme saved for next launch')});
$('uninstall-app').onclick=safe(async()=>{const purge=$('purge-data').checked;const message=purge?'Uninstall Nethernet Desktop AND permanently delete saved profiles, engine files, and tunnel credentials?':'Uninstall Nethernet Desktop? Saved profiles, engine files and tunnel credentials will be kept.';if(!confirm(message))return;if(purge&&!confirm('This also removes your saved tunnel credentials. Continue?'))return;const button=$('uninstall-app');button.disabled=true;button.textContent='Preparing uninstall…';try{await API.UninstallApp(purge);localUIStopped=true;$('state').textContent='Uninstall started';$('substate').textContent='The application will close and the uninstaller will finish removing it.'}catch(e){button.disabled=false;button.textContent='Uninstall application';throw e}});
$('quit-app').onclick=safe(async()=>{if(!confirm('Stop the local Nethernet Desktop UI server?'))return;await API.Quit();localUIStopped=true;$('state').textContent='Local UI stopped';$('substate').textContent='You can close this browser tab.'});
refresh();setInterval(refresh,2500);
