package web

const outputControlsUIScript = `<script>
(function(){
  function ensureOutputControls(){
    var root=document.getElementById('rootInput'); if(!root)return;
    var card=root.closest('.card'); if(!card||document.getElementById('outputRootInput'))return;
    var firstActions=root.closest('.actions');firstActions.id='archiveRootRow';
    var rootSlot=document.createElement('div');rootSlot.id='archiveRootBrowserSlot';firstActions.parentNode.insertBefore(rootSlot,firstActions.nextSibling);
    var row=document.createElement('div');row.style.marginTop='10px';row.innerHTML='<div class="muted" style="margin-bottom:5px">処理結果の保存先</div><div class="actions" id="archiveOutputRow"><input id="outputRootInput" type="text" placeholder="入力フォルダと同じ"><button id="outputBrowseButton">フォルダ選択</button><label class="muted" style="margin-left:8px">既存ファイル <select id="collisionPolicy" style="padding:8px;border:1px solid #aeb6c1;border-radius:7px"><option value="skip">スキップ（推奨）</option><option value="overwrite">上書き</option></select></label></div><div id="archiveOutputBrowserSlot"></div>';
    rootSlot.parentNode.insertBefore(row,rootSlot.nextSibling);
    document.getElementById('outputBrowseButton').onclick=function(){openArchiveBrowser('outputRootInput','archiveOutputBrowserSlot')};
    var scanButton=Array.from(document.querySelectorAll('button')).find(function(b){return b.textContent.trim()==='スキャン'});
    if(scanButton){var actions=scanButton.parentElement;if(!document.getElementById('uncheckAllButton')){var off=document.createElement('button');off.id='uncheckAllButton';off.textContent='全て解除';off.onclick=uncheckAll;actions.insertBefore(off,scanButton.nextSibling);var on=document.createElement('button');on.id='checkAllButton';on.textContent='実行可能を全選択';on.onclick=checkAllExecutable;actions.insertBefore(on,off.nextSibling)}}
  }
  window.uncheckAll=function(){document.querySelectorAll('#plans input[type=checkbox]').forEach(function(x){x.checked=false})};
  window.checkAllExecutable=function(){document.querySelectorAll('#plans input[type=checkbox]').forEach(function(x){if(!x.disabled)x.checked=true})};
  window.openArchiveBrowser=async function(targetId,slotId){var browser=document.getElementById('browser'),slot=document.getElementById(slotId),target=document.getElementById(targetId);if(!browser||!slot||!target)return;slot.appendChild(browser);browser.classList.remove('hidden');window.inlineBrowserTarget=targetId;try{await browse(target.value.trim())}catch(e){await browse('')}};
  window.openOutputBrowser=function(){return openArchiveBrowser('outputRootInput','archiveOutputBrowserSlot')};
  window.openBrowser=function(){return openArchiveBrowser('rootInput','archiveRootBrowserSlot')};
  window.chooseCurrent=function(){var id=window.inlineBrowserTarget||'rootInput',target=document.getElementById(id);if(browserPath&&target)target.value=browserPath;window.inlineBrowserTarget='';closeBrowser()};
  window.loadSettings=async function(){ensureOutputControls();var s=await api('api/settings');document.getElementById('rootInput').value=s.root||'';document.getElementById('outputRootInput').value=s.output_root||s.root||'';document.getElementById('collisionPolicy').value=s.collision_policy==='overwrite'?'overwrite':'skip'};
  window.saveSettings=async function(){ensureOutputControls();var msg=document.getElementById('settingsMsg');msg.className='muted';msg.textContent='保存中...';try{var saved=await api('api/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({root:document.getElementById('rootInput').value,output_root:document.getElementById('outputRootInput').value,collision_policy:document.getElementById('collisionPolicy').value})});document.getElementById('rootInput').value=saved.root;document.getElementById('outputRootInput').value=saved.output_root;document.getElementById('collisionPolicy').value=saved.collision_policy;msg.className='ok';msg.textContent='保存しました';await refreshStatus()}catch(e){msg.className='bad';msg.textContent='保存失敗: '+e.message}};
  ensureOutputControls();
})();
</script>
`
