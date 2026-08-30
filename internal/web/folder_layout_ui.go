package web

import "strings"

const folderLayoutUIScript = `<script id="docExtractorFolderLayoutScript">
(function(){
 function q(id){return document.getElementById(id)}
 function base(){return location.pathname.indexOf('/docExtractor')===0?'/docExtractor/':'/'}
 function exportLayout(root){root=(root||'').trim();if(!root){alert('対象フォルダを選択してください');return}location.href=base()+'api/layout/export?root='+encodeURIComponent(root)}
 function addButton(row,id,label,getRoot){if(!row||q(id))return;var b=document.createElement('button');b.id=id;b.textContent=label;b.onclick=function(){exportLayout(getRoot())};row.appendChild(b)}
 function installArchive(){var output=q('outputRootInput'),root=q('rootInput');if(!root)return;var scan=Array.from(document.querySelectorAll('button')).find(function(b){return b.textContent.trim()==='スキャン'});var row=scan&&scan.parentElement;if(!row)return;addButton(row,'archiveLayoutExport','フォルダ構成TXT出力',function(){return (output&&output.value.trim())||root.value.trim()})}
 function installReprocess(){var card=q('reprocessCard'),root=q('reprocessRoot');if(!card||!root)return;var run=q('reprocessRun'),row=run&&run.closest('.actions');addButton(row,'reprocessLayoutExport','フォルダ構成TXT出力',function(){return root.value.trim()})}
 function installManage(){var card=q('reconcileCard'),output=q('reconcileOutput');if(!card||!output)return;var run=q('reconcileRun'),row=run&&run.closest('.actions');addButton(row,'manageLayoutExport','フォルダ構成TXT出力',function(){return output.value.trim()})}
 function install(){installArchive();installReprocess();installManage()}
 setInterval(install,1000);setTimeout(install,50)
})();
</script>`

func init() {
	indexHTML = strings.Replace(indexHTML, "</body>", folderLayoutUIScript+"</body>", 1)
}
