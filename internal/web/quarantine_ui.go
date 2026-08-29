package web

const quarantineUIScript = `<script>
(function(){
  var qRoots=[],qFiles=[];
  function fmtBytes(n){n=Number(n||0);if(n<1024)return n+' B';var u=['KiB','MiB','GiB','TiB'],i=-1;do{n/=1024;i++}while(n>=1024&&i<u.length-1);return n.toFixed(n>=100?0:n>=10?1:2)+' '+u[i]}
  function activeMode(){var b=document.querySelector('#modeTabs button.primary');return b&&b.dataset?b.dataset.tab:''}
  function ensureQuarantineUI(){
    if(document.getElementById('quarantineCard'))return;
    var rc=document.getElementById('reconcileCard');if(!rc)return;
    var sec=document.createElement('section');sec.id='quarantineCard';sec.className='card mode-card mode-reprocess mode-manage';sec.innerHTML='<div class="actions"><strong>隔離ファイル管理</strong><span class="muted">再整理・統合で隔離された重複/旧版を確認して削除できます。</span></div><div class="actions" style="margin-top:10px"><button id="qRefresh" class="primary">一覧更新</button><button id="qSelectAll">すべて選択</button><button id="qClear">全解除</button><button class="bad" id="qDelete" disabled>選択を削除</button><button class="bad" id="qDeleteAll" disabled>すべて削除</button></div><div id="qSummary" class="muted" style="margin-top:8px">対象フォルダを選択後、「一覧更新」を押してください。</div><div id="qWrap" class="scroll hidden" style="margin-top:10px"><table><thead><tr><th></th><th>隔離ファイル</th><th>容量</th><th>更新日時</th><th>元ルート</th></tr></thead><tbody id="qRows"></tbody></table></div><p class="muted">削除対象は各ライブラリの <code>.docExtractor-duplicates</code> 配下だけです。通常ライブラリのファイルはこの画面から削除できません。削除前に件数と容量を確認します。</p>';
    rc.parentNode.insertBefore(sec,rc.nextSibling);
    document.getElementById('qRefresh').onclick=loadQuarantine;document.getElementById('qSelectAll').onclick=function(){document.querySelectorAll('#qRows input[type=checkbox]').forEach(function(x){x.checked=true});updateQButtons()};document.getElementById('qClear').onclick=function(){document.querySelectorAll('#qRows input[type=checkbox]').forEach(function(x){x.checked=false});updateQButtons()};document.getElementById('qDelete').onclick=function(){deleteQuarantine(false)};document.getElementById('qDeleteAll').onclick=function(){deleteQuarantine(true)};
  }
  function rootsForCurrentMode(){
    var roots=[],mode=activeMode();
    if(mode==='reprocess'){
      var rp=document.getElementById('reprocessRoot');if(rp&&rp.value.trim())roots.push(rp.value.trim());return roots;
    }
    document.querySelectorAll('#reconcileRoots [data-root]').forEach(function(x){var p=x.getAttribute('data-root');if(p)roots.push(p)});
    var one=document.getElementById('reconcileRoot');if(!roots.length&&one&&one.value.trim())roots.push(one.value.trim());
    if(!roots.length){var rp2=document.getElementById('reprocessRoot');if(rp2&&rp2.value.trim())roots.push(rp2.value.trim())}
    return roots;
  }
  function updateQButtons(){var selected=document.querySelectorAll('#qRows input[type=checkbox]:checked').length;document.getElementById('qDelete').disabled=!selected;document.getElementById('qDeleteAll').disabled=!qFiles.length}
  window.loadQuarantine=async function(){ensureQuarantineUI();var roots=rootsForCurrentMode();if(roots.length)qRoots=roots;if(!qRoots.length){document.getElementById('qSummary').textContent='先に再整理または統合の対象フォルダを指定してください。';return}var qs=qRoots.map(function(x){return 'root='+encodeURIComponent(x)}).join('&');try{var r=await api('api/quarantine?'+qs);qFiles=r.files||[];document.getElementById('qSummary').textContent='隔離 '+(r.count||0)+'件 / 合計 '+fmtBytes(r.total_bytes||0)+' / 対象 '+qRoots.length+'フォルダ';var h='';qFiles.forEach(function(x){h+='<tr><td><input type="checkbox" data-path="'+esc(x.path)+'"></td><td>'+esc(x.relative)+'</td><td>'+fmtBytes(x.size)+'</td><td>'+esc(new Date(x.modified).toLocaleString())+'</td><td class="muted">'+esc(x.root)+'</td></tr>'});document.getElementById('qRows').innerHTML=h;document.querySelectorAll('#qRows input').forEach(function(x){x.onchange=updateQButtons});document.getElementById('qWrap').classList.toggle('hidden',!qFiles.length);updateQButtons()}catch(e){document.getElementById('qSummary').textContent='一覧取得失敗: '+e.message}}
  window.deleteQuarantine=async function(all){var paths=[];if(!all)document.querySelectorAll('#qRows input[type=checkbox]:checked').forEach(function(x){paths.push(x.getAttribute('data-path'))});var count=all?qFiles.length:paths.length;if(!count)return;var bytes=0;qFiles.forEach(function(x){if(all||paths.indexOf(x.path)>=0)bytes+=Number(x.size||0)});if(!confirm('隔離済みファイル '+count+'件（'+fmtBytes(bytes)+'）を完全に削除します。\nこの操作は元に戻せません。よろしいですか？'))return;try{var r=await api('api/quarantine/delete',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({roots:qRoots,paths:paths,delete_all:all,confirm:'DELETE QUARANTINED'})});alert('削除 '+(r.deleted||0)+'件 / 解放 '+fmtBytes(r.freed_bytes||0)+((r.errors||[]).length?' / エラー '+r.errors.length:''));await loadQuarantine()}catch(e){alert('削除失敗: '+e.message)}};
  ensureQuarantineUI();
  var oldScan=window.scanReconcile;if(oldScan)window.scanReconcile=async function(){var r=await oldScan.apply(this,arguments);qRoots=rootsForCurrentMode();return r};
})();
</script>
`
