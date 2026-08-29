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
    var cs=Array.isArray(p.candidates)?p.candidates:[],total=Number(p.candidate_count||cs.length||0);
    if(!cs.length)return src;
    var rows=cs.map(function(x){return '<div>• '+esc(x)+'</div>'}).join('');
    var more=total>cs.length?('（代表'+cs.length+'件 / 全'+total+'件）'):(''+cs.length+'件');
    return src+'<details class="muted" style="margin-top:4px"><summary>参照した内部名 '+more+'</summary><div style="padding:5px 0 0 8px;max-width:420px;word-break:break-word">'+rows+'</div></details>';
  }
  function coverageLabel(p){
    var c=p.coverage||{};
    if(c.label)return esc(c.label);
    if(p.has_volume)return '巻 '+esc(p.volume);
    return '-';
  }
  window.scan=async function(){
    var ps=await api('api/scan',{method:'POST'}),h='';
    ps.forEach(function(p){
      var ev=evidenceHTML(p),coverage=coverageLabel(p);
      h+='<tr><td><input type="checkbox" data-name="'+esc(p.name)+'" '+(!p.needs_review&&!p.error?'checked':'')+' '+(p.error?'disabled':'')+'></td>'+
        '<td>'+esc(p.name)+(p.error?'<div class="bad">'+esc(p.error)+'</div>':'')+'</td>'+
        '<td><strong>'+esc(p.series||'-')+'</strong>'+ev+'</td>'+
        '<td>'+coverage+'</td>'+
        '<td class="'+(p.needs_review?'warn':'ok')+'">'+Math.round((p.confidence||0)*100)+'% '+(p.needs_review?'確認':'OK')+'</td>'+
        '<td><span class="pill">'+esc(p.action||'-')+'</span></td></tr>';
    });
    document.getElementById('plans').innerHTML=h;
    var ths=document.querySelectorAll('#plans');
    var table=ths.length?ths[0].closest('table'):null;
    if(table&&table.tHead&&table.tHead.rows.length&&table.tHead.rows[0].cells.length>3)table.tHead.rows[0].cells[3].textContent='収録';
  };
})();
</script>
`
