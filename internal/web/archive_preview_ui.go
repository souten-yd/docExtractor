package web

import "strings"

const archivePreviewUIScript = `<script id="docExtractorArchivePreviewScript">
(function(){
 function q(id){return document.getElementById(id)}
 function base(){return location.pathname.indexOf('/docExtractor')===0?'/docExtractor/':'/'}
 function install(){
  if(q('archiveDryRunExport'))return;
  var scan=Array.from(document.querySelectorAll('button')).find(function(b){return b.textContent.trim()==='スキャン'});
  var row=scan&&scan.parentElement;if(!row)return;
  var b=document.createElement('button');b.id='archiveDryRunExport';b.textContent='解析結果→処理予定TXT';
  b.title='直前に完了したアーカイブ解析結果から、実行予定の出力先をTXTで確認します。ファイルは変更しません。';
  b.onclick=function(){location.href=base()+'api/archive/preview/export'};row.appendChild(b);
 }
 setInterval(install,1000);setTimeout(install,50);
})();
</script>`

func init(){ indexHTML = strings.Replace(indexHTML,"</body>",archivePreviewUIScript+"</body>",1) }
