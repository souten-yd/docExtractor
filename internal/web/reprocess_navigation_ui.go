package web

const reprocessNavigationUIScript = `<script>
(function(){
  var rpOffset=0,rpPageSize=100,rpTotal=0;
  function rpEsc(s){return esc(s)}
  function renderRpItems(r){
    rpTotal=Number(r.total||0);
    var h='';
    (r.items||[]).forEach(function(x){h+='<tr><td>'+rpEsc(x.relative||x.source)+'</td><td><strong>'+rpEsc(x.series||'-')+'</strong>'+(x.has_volume?'<div class="muted">第'+x.volume+'巻</div>':'')+'</td><td>'+rpEsc(x.action)+'</td><td>'+rpEsc(x.reason||x.destination||'-')+'</td></tr>'});
    var rows=document.getElementById('reprocessRows');if(rows)rows.innerHTML=h;
    var wrap=document.getElementById('reprocessTableWrap');if(wrap)wrap.classList.toggle('hidden',!rpTotal);
    var start=rpTotal?rpOffset+1:0,end=Math.min(rpOffset+rpPageSize,rpTotal),pages=Math.max(1,Math.ceil(rpTotal/rpPageSize)),page=rpTotal?Math.floor(rpOffset/rpPageSize)+1:1;
    var range=document.getElementById('reprocessPage');if(range)range.textContent=rpTotal?(start+'–'+end+' / '+rpTotal):'0件';
    var inp=document.getElementById('reprocessPageInput');if(inp){inp.max=String(pages);inp.value=String(page)}
    var pc=document.getElementById('reprocessPageCount');if(pc)pc.textContent='/ '+pages+' ページ';
    var prev=document.getElementById('reprocessPrev'),next=document.getElementById('reprocessNext');
    if(prev)prev.disabled=rpOffset===0;if(next)next.disabled=rpOffset+rpPageSize>=rpTotal;
  }
  async function loadRp(offset){
    offset=Math.max(0,Number(offset)||0);if(rpTotal&&offset>=rpTotal)offset=Math.max(0,(Math.ceil(rpTotal/rpPageSize)-1)*rpPageSize);rpOffset=offset;
    try{var r=await api('api/reconcile/async/items?mode=reprocess&offset='+rpOffset+'&limit='+rpPageSize);renderRpItems(r)}catch(e){var p=document.getElementById('reprocessPage');if(p)p.textContent='ページ取得失敗: '+e.message}
  }
  async function refreshRpMeta(){try{var s=await api('api/reconcile/async/status?mode=reprocess');rpTotal=Number(s.items||0);var pages=Math.max(1,Math.ceil(rpTotal/rpPageSize)),pc=document.getElementById('reprocessPageCount');if(pc)pc.textContent='/ '+pages+' ページ';var inp=document.getElementById('reprocessPageInput');if(inp)inp.max=String(pages)}catch(e){}}
  function installPageControls(){
    var wrap=document.getElementById('reprocessTableWrap');if(!wrap||document.getElementById('reprocessPageInput'))return;
    var nav=wrap.querySelector('.actions');if(!nav)return;
    var label=document.createElement('span');label.className='actions reprocess-page-jump';label.style.marginLeft='auto';label.innerHTML='<span class="muted">ページ</span><input id="reprocessPageInput" type="number" min="1" value="1" inputmode="numeric" style="width:78px;min-width:0;padding:7px 8px"><span id="reprocessPageCount" class="muted">/ - ページ</span><button id="reprocessPageGo">移動</button>';
    nav.appendChild(label);
    var go=function(){var inp=document.getElementById('reprocessPageInput'),page=Math.max(1,parseInt(inp.value||'1',10)||1);loadRp((page-1)*rpPageSize)};
    document.getElementById('reprocessPageGo').onclick=go;document.getElementById('reprocessPageInput').onkeydown=function(e){if(e.key==='Enter'){e.preventDefault();go()}};
    var prev=document.getElementById('reprocessPrev'),next=document.getElementById('reprocessNext');if(prev)prev.onclick=function(){loadRp(Math.max(0,rpOffset-rpPageSize))};if(next)next.onclick=function(){loadRp(rpOffset+rpPageSize)};
    refreshRpMeta();
  }
  function installQuarantineShortcut(){
    var run=document.getElementById('reprocessRun');if(!run||document.getElementById('reprocessQuarantine'))return;
    var b=document.createElement('button');b.id='reprocessQuarantine';b.textContent='隔離ファイル管理';b.onclick=async function(){if(window.loadQuarantine)await window.loadQuarantine();var c=document.getElementById('quarantineCard');if(c)c.scrollIntoView({behavior:'smooth',block:'start'})};run.parentNode.appendChild(b);
    var note=document.createElement('span');note.id='reprocessRunHint';note.className='muted';note.textContent=run.disabled?'解析完了後に「再整理を実行」が有効になります。':'';run.parentNode.appendChild(note);
    var ob=new MutationObserver(function(){note.textContent=run.disabled?'解析完了後に「再整理を実行」が有効になります。':'再整理を実行できます。'});ob.observe(run,{attributes:true,attributeFilter:['disabled']});
  }
  function install(){installPageControls();installQuarantineShortcut();var scan=document.getElementById('reprocessScan');if(scan&&!scan.dataset.pageReset){scan.dataset.pageReset='1';scan.addEventListener('click',function(){rpOffset=0;rpTotal=0;var inp=document.getElementById('reprocessPageInput');if(inp)inp.value='1'})}}
  install();setTimeout(install,0);
  var tabs=document.getElementById('modeTabs');if(tabs)tabs.addEventListener('click',function(e){if(e.target&&e.target.dataset&&e.target.dataset.tab==='reprocess'){setTimeout(function(){install();refreshRpMeta()},0)}});
})();
</script>
`
