package web

import "strings"

func renderIndexHTML() string {
	if !strings.Contains(indexHTML, "</body>") {
		return indexHTML
	}
	return strings.Replace(indexHTML, "</body>", internalNameUIScript+"</body>", 1)
}

const internalNameUIScript = `<script>
(function(){
  function sourceLabel(s){
    return ({'nested-archive':'内部ZIP/RAR','top-directory':'内部フォルダ','named-image':'画像ファイル名','outer-filename':'外側ファイル名'})[s]||s||'外側ファイル名';
  }
  function evidenceHTML(p){
    var src='<div class="muted">判定元: '+esc(sourceLabel(p.name_source))+'</div>';
    var cs=Array.isArray(p.candidates)?p.candidates:[];
    if(!cs.length)return src;
    var rows=cs.map(function(x){return '<div>• '+esc(x)+'</div>'}).join('');
    return src+'<details class="muted" style="margin-top:4px"><summary>参照した内部名 '+cs.length+'件</summary><div style="padding:5px 0 0 8px;max-width:380px;word-break:break-word">'+rows+'</div></details>';
  }
  window.scan=async function(){
    var ps=await api('api/scan',{method:'POST'}),h='';
    ps.forEach(function(p){
      var ev=evidenceHTML(p);
      h+='<tr><td><input type="checkbox" data-name="'+esc(p.name)+'" '+(!p.needs_review&&!p.error?'checked':'')+' '+(p.error?'disabled':'')+'></td>'+
        '<td>'+esc(p.name)+(p.error?'<div class="bad">'+esc(p.error)+'</div>':'')+'</td>'+
        '<td><strong>'+esc(p.series||'-')+'</strong>'+ev+'</td>'+
        '<td>'+(p.has_volume?esc(p.volume):'-')+'</td>'+
        '<td class="'+(p.needs_review?'warn':'ok')+'">'+Math.round((p.confidence||0)*100)+'% '+(p.needs_review?'確認':'OK')+'</td>'+
        '<td><span class="pill">'+esc(p.action||'-')+'</span></td></tr>';
    });
    document.getElementById('plans').innerHTML=h;
  };
})();
</script>
`
