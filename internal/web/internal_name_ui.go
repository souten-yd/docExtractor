package web

import "strings"

func renderIndexHTML() string {
	if !strings.Contains(indexHTML, "</body>") { return indexHTML }
	return strings.Replace(indexHTML, "</body>", internalNameUIScript+outputControlsUIScript+reconcileUIScript+quarantineUIScript+tabsUIScript+reprocessNavigationUIScript+"</body>", 1)
}

const internalNameUIScript = `<script>
(function(){
  function sourceLabel(s){return ({'nested-archive':'内部ZIP/RAR','top-directory':'内部フォルダ','named-image':'画像ファイル名','outer-filename':'外側ファイル名'})[s]||s||'外側ファイル名';}
  function evidenceHTML(p){
    var src='<div class="muted">判定元: '+esc(sourceLabel(p.name_source))+'</div>';
    if(p.multipart){var parts=Array.isArray(p.parts)?p.parts:[],count=Number(p.part_count||parts.length||0);src+='<div class="muted" style="margin-top:2px"><strong>分割RAR: '+count+'パート</strong>（part1を代表として処理）</div>';if(parts.length)src+='<details class="muted"><summary>分割ファイルを確認</summary><div style="padding-left:8px">'+parts.map(function(x){return '<div>• '+esc(x)+'</div>';}).join('')+'</div></details>'}
    var cl=p.cluster||{};if(cl.canonical){var aliases=Array.isArray(cl.aliases)?cl.aliases:[];src+='<div class="muted" style="margin-top:2px">統合: '+esc(cl.canonical)+(cl.reason?' / '+esc(cl.reason):'')+(cl.score?(' '+Math.round(cl.score*100)+'%'):'')+'</div>';if(aliases.length>1)src+='<details class="muted"><summary>別表記 '+aliases.length+'件</summary><div style="padding-left:8px">'+aliases.map(function(x){return '<div>• '+esc(x)+'</div>';}).join('')+'</div></details>'}
    var cs=Array.isArray(p.candidates)?p.candidates:[],total=Number(p.candidate_count||cs.length||0);if(cs.length){var rows=cs.map(function(x){return '<div>• '+esc(x)+'</div>';}).join('');var more=total>cs.length?('（代表'+cs.length+'件 / 全'+total+'件）'):(''+cs.length+'件');src+='<details class="muted" style="margin-top:4px"><summary>参照した内部名 '+more+'</summary><div style="padding:5px 0 0 8px;max-width:420px;word-break:break-word">'+rows+'</div></details>'}return src;
  }
  function coverageLabel(p){var c=p.coverage||{};if(c.label)return esc(c.label);if(p.has_volume)return '巻 '+esc(p.volume);return '-';}
  async function registerAlias(alias,canonical){alias=(alias||'').trim();canonical=(canonical||'').trim();var a=prompt('別名として登録する名称',alias);if(a===null)return;a=a.trim();if(!a)return;var c=prompt('統一先のシリーズ名',canonical);if(c===null)return;c=c.trim();if(!c)return;try{await api('api/aliases',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({alias:a,canonical:c})});await scan();alert('別名を保存しました');}catch(e){alert('別名保存失敗: '+e.message)}}
  window.saveSeriesAlias=registerAlias;
  window.scan=async function(){var ps=await api('api/scan',{method:'POST'}),h='';ps.forEach(function(p){var ev=evidenceHTML(p),coverage=coverageLabel(p),cl=p.cluster||{},alias='';if(Array.isArray(cl.aliases)){for(var i=0;i<cl.aliases.length;i++){if(cl.aliases[i]!==p.series){alias=cl.aliases[i];break;}}}var aliasBtn='<div style="margin-top:5px"><button style="padding:3px 7px;font-size:12px" onclick="saveSeriesAlias('+JSON.stringify(alias||p.series)+','+JSON.stringify(p.series||'')+')">別名登録</button></div>';var fileLabel=esc(p.name)+(p.multipart?'<div class="muted">分割RAR '+Number(p.part_count||0)+'パート</div>':'');var disabled=p.error||p.skipped,checked=!p.needs_review&&!disabled;var state=p.skipped?'<div class="muted">既存出力あり: スキップ</div>':'';h+='<tr><td><input type="checkbox" data-name="'+esc(p.name)+'" '+(checked?'checked':'')+' '+(disabled?'disabled':'')+'></td><td>'+fileLabel+(p.error?'<div class="bad">'+esc(p.error)+'</div>':'')+state+'</td><td><strong>'+esc(p.series||'-')+'</strong>'+ev+aliasBtn+'</td><td>'+coverage+'</td><td class="'+(p.needs_review?'warn':'ok')+'">'+Math.round((p.confidence||0)*100)+'% '+(p.needs_review?'確認':'OK')+'</td><td><span class="pill">'+esc(p.action||'-')+'</span></td></tr>'});document.getElementById('plans').innerHTML=h;var body=document.getElementById('plans'),table=body?body.closest('table'):null;if(table&&table.tHead&&table.tHead.rows.length&&table.tHead.rows[0].cells.length>3)table.tHead.rows[0].cells[3].textContent='収録';};
})();
</script>
`
