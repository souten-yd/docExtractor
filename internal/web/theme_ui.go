package web

import "strings"

const responsiveThemeStyle = `<style id="docExtractorThemeStyle">
html{color-scheme:light dark}body,button,input,.card,.metric,.browser,.update-box,table,pre{transition:background-color .15s,color .15s,border-color .15s}
html[data-theme="dark"] body{background:#080b10;color:#e6edf3}
html[data-theme="dark"] .card,html[data-theme="dark"] .metric,html[data-theme="dark"] .update-box{background:#11161d;border-color:#29313b}
html[data-theme="dark"] button,html[data-theme="dark"] input[type=text]{background:#171d25;color:#e6edf3;border-color:#3a4654}
html[data-theme="dark"] button:hover{background:#202833}
html[data-theme="dark"] .primary{background:#e6edf3;color:#11161d;border-color:#e6edf3}
html[data-theme="dark"] .accent{background:#0f766e;color:#fff;border-color:#20a79a}
html[data-theme="dark"] .muted,html[data-theme="dark"] .metric .label{color:#9ba7b4}
html[data-theme="dark"] th,html[data-theme="dark"] td{border-bottom-color:#27303a}
html[data-theme="dark"] .pill{background:#252e39;color:#dbe5ef}
html[data-theme="dark"] .state-running{background:#17325a}html[data-theme="dark"] .state-succeeded{background:#123c2c}html[data-theme="dark"] .state-failed{background:#4a2020}html[data-theme="dark"] .state-queued{background:#29313a}
html[data-theme="dark"] .browser{border-color:#303a46;background:#0d1218}html[data-theme="dark"] .progress{background:#27303a}html[data-theme="dark"] a{color:#79b8ff}
#modeTabs .actions{overflow-x:auto;flex-wrap:nowrap;-webkit-overflow-scrolling:touch}#modeTabs button{white-space:nowrap}
@media(max-width:700px){
 .wrap{padding:9px}.card{padding:12px;margin:10px 0;border-radius:9px}header{align-items:flex-start}.header-actions{flex-wrap:wrap;justify-content:flex-end}.header-actions button{padding:7px 9px}
 .metrics{grid-template-columns:repeat(2,minmax(0,1fr));gap:7px}.metric{padding:9px}.metric .value{font-size:17px}.metric .label{font-size:11px}
 .actions{align-items:stretch}.actions input[type=text]{min-width:0;width:100%;flex-basis:100%}#reprocessPickerRow button,#reconcileOutputRow button{width:100%}
 .browser-list{max-height:42vh}.scroll{-webkit-overflow-scrolling:touch}table{min-width:720px;font-size:12px}th,td{padding:6px 5px}
 #reprocessTableWrap table,#reconcileTableWrap table{min-width:820px}fieldset{min-width:0;overflow-wrap:anywhere}.brand h1{font-size:20px}.brandmark{flex:0 0 auto}
 #modeTabs{position:sticky;top:0;z-index:20;padding:6px!important;background:inherit}#modeTabs .actions{gap:5px}#modeTabs button{padding:8px 10px}
}
@media(max-width:390px){.metrics{grid-template-columns:1fr 1fr}.metric .value{font-size:15px}.header-actions{gap:4px}.header-actions button{font-size:12px}}
</style>`

const responsiveThemeScript = `<script id="docExtractorThemeScript">
(function(){
 function preferred(){try{var s=localStorage.getItem('docExtractorTheme');if(s==='light'||s==='dark')return s}catch(e){}return window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'}
 function apply(v){document.documentElement.dataset.theme=(v==='system'?preferred():v);var b=document.getElementById('themeToggle');if(b){var saved='system';try{saved=localStorage.getItem('docExtractorTheme')||'system'}catch(e){};b.textContent=saved==='dark'?'☀ 明るく':saved==='light'?'◐ 自動':'☾ 黒テーマ'}}
 function cycle(){var v='system';try{v=localStorage.getItem('docExtractorTheme')||'system'}catch(e){};v=v==='system'?'dark':v==='dark'?'light':'system';try{localStorage.setItem('docExtractorTheme',v)}catch(e){};apply(v)}
 apply('system');if(window.matchMedia){var mq=window.matchMedia('(prefers-color-scheme: dark)');if(mq.addEventListener)mq.addEventListener('change',function(){try{if((localStorage.getItem('docExtractorTheme')||'system')==='system')apply('system')}catch(e){apply('system')}})}
 function fmt(ms){ms=Number(ms||0);if(ms<=0)return'-';var s=Math.round(ms/1000);if(s<60)return s+'秒';var m=Math.floor(s/60);if(m<60)return m+'分 '+(s%60)+'秒';var h=Math.floor(m/60);return h+'時間 '+(m%60)+'分'}
 function install(){
  var ha=document.querySelector('.header-actions');if(ha&&!document.getElementById('themeToggle')){var b=document.createElement('button');b.id='themeToggle';b.onclick=cycle;ha.insertBefore(b,ha.firstChild);apply('system')}
  var tabs=document.getElementById('modeTabs');if(tabs&&!document.getElementById('archiveBatchProgress')){var sec=document.createElement('section');sec.id='archiveBatchProgress';sec.className='card mode-card mode-archive';sec.innerHTML='<div class="actions"><strong>アーカイブ処理進捗</strong><span class="muted" id="archiveBatchMsg">待機中</span></div><div class="metrics"><div class="metric"><div class="label">完了 / 全数</div><div class="value" id="archiveBatchCount">0 / 0</div></div><div class="metric"><div class="label">進捗</div><div class="value" id="archiveBatchPct">-</div></div><div class="metric"><div class="label">経過時間</div><div class="value" id="archiveBatchElapsed">-</div></div><div class="metric"><div class="label">推定残り</div><div class="value" id="archiveBatchEta">-</div></div><div class="metric"><div class="label">速度</div><div class="value" id="archiveBatchRate">-</div></div></div><div class="progress"><div id="archiveBatchBar"></div></div>';tabs.parentNode.insertBefore(sec,tabs.nextSibling)}
  updateArchiveBatch();restoreAsync('reprocess','reprocessProgress');restoreAsync('manage','manageProgress')
 }
 async function updateArchiveBatch(){if(!document.getElementById('archiveBatchProgress'))return;try{var js=await api('api/jobs');var total=js.length,terminal=0,equiv=0,first=0;js.forEach(function(j){var t=Date.parse(j.created_at||j.started_at||'');if(t&&(!first||t<first))first=t;if(['succeeded','failed','cancelled'].indexOf(j.state)>=0){terminal++;equiv++}else if(j.state==='running'){equiv+=Number(j.progress||0)}});var elapsed=first?Date.now()-first:0,ratev=elapsed>0?equiv/(elapsed/1000):0,eta=ratev>0&&equiv<total?(total-equiv)/ratev*1000:0,pct=total?equiv/total*100:0;document.getElementById('archiveBatchCount').textContent=terminal+' / '+total;document.getElementById('archiveBatchPct').textContent=total?Math.round(pct)+'%':'-';document.getElementById('archiveBatchElapsed').textContent=fmt(elapsed);document.getElementById('archiveBatchEta').textContent=eta?fmt(eta):(total&&terminal===total?'0秒':'-');document.getElementById('archiveBatchRate').textContent=ratev?ratev.toFixed(2)+' 件/秒':'-';document.getElementById('archiveBatchBar').style.width=Math.min(100,pct)+'%';document.getElementById('archiveBatchMsg').textContent=total?(terminal===total?'完了':'サーバ側で処理中'):'待機中'}catch(e){}setTimeout(updateArchiveBatch,2000)}
 function renderAsync(id,s){var q=function(x){return document.getElementById(id+x)},t=Number(s.total||0),c=Number(s.completed||0),p=t?Math.min(100,c/t*100):0;if(!q('Title'))return;var names={starting:'開始中',counting:'全数確認中',inspecting:'アーカイブ解析中',clustering:'シリーズ統合中',duplicates:'重複確認中',variants:'同巻判定中',executing:'整理実行中',done:'完了',failed:'失敗'};q('Title').textContent=names[s.phase]||s.phase||'待機中';q('Msg').textContent=s.message||'';q('Count').textContent=c+' / '+(t||'-');q('Pct').textContent=t?Math.round(p)+'%':'-';q('Elapsed').textContent=fmt(s.elapsed_ms);q('Eta').textContent=s.estimated_remaining_ms?fmt(s.estimated_remaining_ms):(s.phase==='done'?'0秒':'-');q('Rate').textContent=s.items_per_second?Number(s.items_per_second).toFixed(2)+' 件/秒':'-';q('Bar').style.width=p+'%'}
 async function restoreAsync(mode,id){if(!document.getElementById(id))return;try{var s=await api('api/reconcile/async/status?mode='+mode);if(!s.id)return;renderAsync(id,s);if(s.running){setTimeout(function(){restoreAsync(mode,id)},1000)}else{var target=document.getElementById(mode==='manage'?'reconcileSummary':'reprocessSummary');if(target&&s.error){target.className='bad';target.textContent='処理失敗: '+s.error}else if(target&&s.summary&&s.items){var x=s.summary;target.textContent='対象 '+(x.files||0)+'件 / 移動 '+(x.move||0)+' / 完全重複 '+(x.duplicates||0)+' / 旧版隔離 '+(x.superseded||0)+' / 要選択 '+(x.review||0)+' / エラー '+(x.errors||0)}}}catch(e){}}
 setTimeout(install,0)
})();
</script>`

func init() {
	indexHTML = strings.Replace(indexHTML, "</head>", responsiveThemeStyle+"</head>", 1)
	indexHTML = strings.Replace(indexHTML, "</body>", responsiveThemeScript+"</body>", 1)
}
