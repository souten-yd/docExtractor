package web

const tabsUIScript = `<script>
(function(){
  function makeTabBar(){
    if(document.getElementById('modeTabs'))return;
    var wrap=document.querySelector('.wrap');if(!wrap)return;
    var metrics=document.querySelector('.metrics');
    var bar=document.createElement('div');bar.id='modeTabs';bar.className='card';bar.style.padding='8px';bar.innerHTML='<div class="actions"><button class="primary" data-tab="archive">アーカイブ処理</button><button data-tab="reprocess">既存ファイル再整理</button><button data-tab="manage">統合ファイル管理</button></div>';
    if(metrics)metrics.parentNode.insertBefore(bar,metrics.nextSibling);else wrap.insertBefore(bar,wrap.firstChild);
    Array.from(bar.querySelectorAll('button')).forEach(function(b){b.onclick=function(){showTab(b.dataset.tab)}});
  }
  function ensureReprocessCard(){
    if(document.getElementById('reprocessCard'))return;
    var manage=document.getElementById('reconcileCard');if(!manage)return;
    var sec=document.createElement('section');sec.id='reprocessCard';sec.className='card mode-card mode-reprocess';
    sec.innerHTML='<div class="actions"><strong>既存ファイル再整理</strong><span class="muted">旧 MangaOrganize.py などで整理済みの1ライブラリを、その場所で最新の判定形式へ揃えます。</span></div><div style="margin-top:10px"><div class="muted">対象フォルダ</div><div class="actions" id="reprocessPickerRow"><input id="reprocessRoot" type="text" placeholder="/share/Comics"><button id="reprocessBrowse">フォルダ選択</button></div><div id="reprocessBrowserSlot"></div></div><div class="actions" style="margin-top:10px"><button class="primary" id="reprocessScan">解析</button><button class="accent" id="reprocessRun" disabled>再整理を実行</button></div><div id="reprocessSummary" class="muted" style="margin-top:8px"></div><div id="reprocessTableWrap" class="scroll hidden" style="margin-top:10px"><table><thead><tr><th>現在</th><th>シリーズ/巻</th><th>処理</th><th>移動先/理由</th></tr></thead><tbody id="reprocessRows"></tbody></table></div><p class="muted">単一ライブラリ内のフォルダ名揺らぎ・重複・シリーズ表記を再整理します。完全重複や旧版は削除せず隔離します。</p>';
    manage.parentNode.insertBefore(sec,manage);
    document.getElementById('reprocessBrowse').onclick=function(){openBrowserInline('reprocessRoot','reprocessBrowserSlot')};
    document.getElementById('reprocessScan').onclick=scanReprocess;document.getElementById('reprocessRun').onclick=runReprocess;
  }
  async function scanReprocess(){var root=document.getElementById('reprocessRoot').value.trim();if(!root){alert('対象フォルダを選択してください');return}var sum=document.getElementById('reprocessSummary');sum.className='muted';sum.textContent='解析中…';document.getElementById('reprocessRun').disabled=true;try{var r=await api('api/reconcile/scan',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({root:root,output_root:root})});window.lastReprocessReport=r;var s=r.summary||{};sum.textContent='対象 '+(s.files||0)+'件 / 移動 '+(s.move||0)+' / 完全重複 '+(s.duplicates||0)+' / 旧版隔離 '+(s.superseded||0)+' / そのまま '+(s.keep||0)+' / 要選択 '+(s.review||0)+' / 衝突 '+(s.conflicts||0)+' / エラー '+(s.errors||0);var h='';(r.items||[]).forEach(function(x){h+='<tr><td>'+esc(x.relative||x.source)+'</td><td><strong>'+esc(x.series||'-')+'</strong>'+(x.has_volume?'<div class="muted">第'+x.volume+'巻</div>':'')+'</td><td>'+esc(x.action)+'</td><td>'+esc(x.reason||x.destination||'-')+'</td></tr>'});document.getElementById('reprocessRows').innerHTML=h;document.getElementById('reprocessTableWrap').classList.remove('hidden');document.getElementById('reprocessRun').disabled=(s.review||0)>0||((s.move||0)+(s.duplicates||0)+(s.superseded||0)===0)}catch(e){sum.className='bad';sum.textContent='解析失敗: '+e.message}}
  async function runReprocess(){var root=document.getElementById('reprocessRoot').value.trim(),rep=window.lastReprocessReport;if(!root||!rep)return;if((rep.choices||[]).length){alert('自動判定できない項目があります。複数候補の選択が必要な場合は「統合ファイル管理」タブを使用してください。');return}if(!confirm('このライブラリを最新形式で再整理します。旧版・重複は削除せず隔離します。よろしいですか？'))return;var sum=document.getElementById('reprocessSummary');sum.textContent='再整理中…';try{var r=await api('api/reconcile/execute',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({root:root,output_root:root,selections:{}})});sum.textContent='完了: 移動 '+(r.moved||0)+' / 隔離 '+(r.quarantined||0)+' / スキップ '+(r.skipped||0)+' / エラー '+((r.errors||[]).length);await scanReprocess()}catch(e){sum.className='bad';sum.textContent='実行失敗: '+e.message}}
  window.showTab=function(name){
    document.querySelectorAll('#modeTabs button').forEach(function(b){b.classList.toggle('primary',b.dataset.tab===name)});
    document.querySelectorAll('.mode-card').forEach(function(c){c.classList.add('hidden')});
    document.querySelectorAll('.mode-'+name).forEach(function(c){c.classList.remove('hidden')});
    try{localStorage.setItem('docExtractorTab',name)}catch(e){}
  };
  window.openBrowserInline=async function(targetId,slotId){
    var browser=document.getElementById('browser'),slot=document.getElementById(slotId),target=document.getElementById(targetId);if(!browser||!slot||!target)return;
    slot.appendChild(browser);browser.classList.remove('hidden');window.inlineBrowserTarget=targetId;
    try{await browse(target.value.trim())}catch(e){await browse('')}
  };
  var oldChoose=window.chooseCurrent;window.chooseCurrent=function(){if(window.inlineBrowserTarget&&browserPath){var el=document.getElementById(window.inlineBrowserTarget);if(el)el.value=browserPath;window.inlineBrowserTarget='';closeBrowser();return}return oldChoose()};
  function classifyExisting(){
    var cards=Array.from(document.querySelectorAll('.wrap > .card'));
    cards.forEach(function(c){if(c.id==='modeTabs'||c.id==='updateCard'||c.id==='logcard'||c.id==='reprocessCard'||c.id==='reconcileCard'||c.id==='quarantineCard')return;var txt=c.textContent||'';if(txt.indexOf('ジョブ更新')>=0||txt.indexOf('スキャン')>=0||txt.indexOf('設定')>=0)c.classList.add('mode-card','mode-archive')});
    var rec=document.getElementById('reconcileCard');if(rec)rec.classList.add('mode-card','mode-manage');var q=document.getElementById('quarantineCard');if(q)q.classList.add('mode-card','mode-manage');
  }
  makeTabBar();ensureReprocessCard();classifyExisting();var tab='archive';try{tab=localStorage.getItem('docExtractorTab')||'archive'}catch(e){};if(['archive','reprocess','manage'].indexOf(tab)<0)tab='archive';showTab(tab);
})();
</script>
`
