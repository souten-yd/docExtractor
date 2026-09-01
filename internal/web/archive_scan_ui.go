package web

const archiveScanUIScript = `<script>
(function(){
  var polling=false;
  function fmtMS(ms){ms=Number(ms||0);if(ms<=0)return '-';var s=Math.round(ms/1000);if(s<60)return s+'秒';var m=Math.floor(s/60);if(m<60)return m+'分 '+(s%60)+'秒';var h=Math.floor(m/60);return h+'時間 '+(m%60)+'分'}
  function ensureProgress(){
    if(document.getElementById('archiveScanProgress'))return;
    var plans=document.getElementById('plans'),table=plans?plans.closest('table'):null,card=table?table.closest('.card'):null;if(!card)return;
    var d=document.createElement('div');d.id='archiveScanProgress';d.style.cssText='margin:10px 0;padding:10px;border:1px solid #e0e4e8;border-radius:8px';d.innerHTML='<div class="actions"><strong id="archiveScanTitle">待機中</strong><span id="archiveScanMessage" class="muted"></span></div><div class="metrics" style="margin:8px 0"><div class="metric"><div class="label">完了 / 全数</div><div class="value" id="archiveScanCount">0 / -</div></div><div class="metric"><div class="label">進捗</div><div class="value" id="archiveScanPct">-</div></div><div class="metric"><div class="label">経過時間</div><div class="value" id="archiveScanElapsed">-</div></div><div class="metric"><div class="label">速度</div><div class="value" id="archiveScanRate">-</div></div></div><div class="progress"><div id="archiveScanBar"></div></div><div id="archiveScanCurrent" class="muted" style="word-break:break-all"></div><div id="archiveScanError" class="bad"></div><p class="muted" style="margin-bottom:0">ブラウザを閉じてもスキャンはサーバー側で継続します。ZIP/RAR内部のネストも再帰的に解析します。</p>';
    table.parentNode.insertBefore(d,table);
  }
  function label(p){return ({idle:'待機中',starting:'開始中',inspecting:'アーカイブ解析中',clustering:'シリーズ表記統合中',done:'完了',failed:'失敗'})[p]||p||'待機中'}
  function render(s){ensureProgress();var t=Number(s.total||0),c=Number(s.completed||0),pct=t?Math.min(100,c/t*100):0;document.getElementById('archiveScanTitle').textContent=label(s.phase);document.getElementById('archiveScanMessage').textContent=s.message||'';document.getElementById('archiveScanCount').textContent=c+' / '+(t||'-');document.getElementById('archiveScanPct').textContent=t?Math.round(pct)+'%':'-';document.getElementById('archiveScanElapsed').textContent=fmtMS(s.elapsed_ms);document.getElementById('archiveScanRate').textContent=s.items_per_second?Number(s.items_per_second).toFixed(2)+' 件/秒':'-';document.getElementById('archiveScanBar').style.width=pct+'%';document.getElementById('archiveScanCurrent').textContent=s.current?('現在: '+s.current):'';document.getElementById('archiveScanError').textContent=s.error||'';}
  async function getStatus(){return api('api/scan?op=status',{method:'POST'})}
  async function loadItems(){var ps=await api('api/scan?op=items',{method:'POST'});if(window.renderArchivePlans)window.renderArchivePlans(ps)}
  async function poll(){if(polling)return;polling=true;try{while(true){var s=await getStatus();render(s);if(!s.running){if(s.phase==='done')await loadItems();break}await new Promise(function(r){setTimeout(r,800)})}}catch(e){ensureProgress();document.getElementById('archiveScanError').textContent='状態取得失敗: '+e.message}finally{polling=false}}
  window.scan=async function(){ensureProgress();try{if(window.ensureArchiveSettingsSaved)await window.ensureArchiveSettingsSaved();var s=await api('api/scan?op=start',{method:'POST'});render(s);await poll()}catch(e){document.getElementById('archiveScanError').textContent='スキャン開始失敗: '+e.message}};
  ensureProgress();
  setTimeout(async function(){try{var s=await getStatus();render(s);if(s.running)await poll();else if(s.phase==='done')await loadItems()}catch(e){}},0);
})();
</script>
`
