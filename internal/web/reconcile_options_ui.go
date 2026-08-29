package web

import "strings"

// Keep this script in the base HTML before the dynamically injected mode UI.
// It wraps the shared api() helper once, then installs controls after the tab
// scripts have created their cards.
func init() {
	if strings.Contains(indexHTML, "</body>") {
		indexHTML = strings.Replace(indexHTML, "</body>", reconcileOptionsUIScript+"</body>", 1)
	}
}

const reconcileOptionsUIScript = `<script>
(function(){
  var baseApi=window.api;
  if(typeof baseApi==='function'&&!window.__docExtractorReconcileOptionsWrapped){
    window.__docExtractorReconcileOptionsWrapped=true;
    window.api=async function(path,opt){
      if(String(path||'').replace(/^\/+/, '')==='api/reconcile/async/start'&&opt&&opt.body){
        try{
          var body=JSON.parse(opt.body),id=body.mode==='reprocess'?'reprocessIncludeQuarantine':'manageIncludeQuarantine',cb=document.getElementById(id);
          body.include_quarantine=!!(cb&&cb.checked);
          opt=Object.assign({},opt,{body:JSON.stringify(body)});
        }catch(e){}
      }
      return baseApi(path,opt);
    };
  }

  function optionHTML(id){return '<label class="reconcile-quarantine-option" style="display:flex;gap:8px;align-items:flex-start;margin-top:10px;padding:10px;border:1px solid #dfe3e8;border-radius:8px"><input id="'+id+'" type="checkbox" style="margin-top:3px"><span><strong>隔離フォルダも解析する</strong><span class="muted" style="display:block;margin-top:2px">.docExtractor-duplicates 内も再判定します。選ばれたファイルは通常ライブラリへ戻る場合があります。解析だけでは削除・移動しません。</span></span></label>'}
  function installReprocess(){
    var card=document.getElementById('reprocessCard');if(!card||document.getElementById('reprocessIncludeQuarantine'))return;
    var row=document.getElementById('reprocessPickerRow');if(!row)return;
    var holder=document.createElement('div');holder.innerHTML=optionHTML('reprocessIncludeQuarantine');row.parentNode.insertBefore(holder.firstChild,row.nextSibling);
  }
  function installManage(){
    var card=document.getElementById('reconcileCard');if(!card||document.getElementById('manageIncludeQuarantine'))return;
    var out=document.getElementById('reconcileOutput'),anchor=out&&out.parentNode?out.parentNode:null;
    var holder=document.createElement('div');holder.innerHTML=optionHTML('manageIncludeQuarantine');
    if(anchor&&anchor.parentNode)anchor.parentNode.insertBefore(holder.firstChild,anchor.nextSibling);else card.appendChild(holder.firstChild);
  }
  function install(){installReprocess();installManage()}
  setTimeout(install,0);setTimeout(install,250);
  document.addEventListener('click',function(e){if(e.target&&e.target.dataset&&e.target.dataset.tab)setTimeout(install,0)});
})();
</script>
`
