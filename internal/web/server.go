package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/souten-yd/docExtractor/internal/diagnostics"
	"github.com/souten-yd/docExtractor/internal/jobs"
	"github.com/souten-yd/docExtractor/internal/organizer"
	appsettings "github.com/souten-yd/docExtractor/internal/settings"
)

type Server struct {
	Organizer   *organizer.Organizer
	Jobs        *jobs.Manager
	Diagnostics *diagnostics.Manager
	Settings    *appsettings.Store
	BrowseRoot  string
	Version     string
}

type submitRequest struct {
	Names       []string `json:"names"`
	AllowReview bool     `json:"allow_review"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.updateSettings)
	mux.HandleFunc("GET /api/directories", s.listDirectories)
	mux.HandleFunc("POST /api/scan", s.scan)
	mux.HandleFunc("GET /api/jobs", s.listJobs)
	mux.HandleFunc("GET /api/jobs/{jobID}", s.getJob)
	mux.HandleFunc("POST /api/jobs", s.submitJobs)
	mux.HandleFunc("POST /api/jobs/{jobID}/cancel", s.cancelJob)
	DiagnosticsHandler{
		Manager: s.Diagnostics, Version: s.Version,
		ArchiveRoot: s.Organizer.Root, Workers: s.Jobs.Workers(),
	}.Register(mux)

	// QTS can expose the app through QPKG_PROXY_PATH=/docExtractor. Some QTS
	// versions strip the prefix before proxying and some integrations may not,
	// so accept both forms without changing the API implementation.
	router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/docExtractor" {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			mux.ServeHTTP(w, r2)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/docExtractor/") {
			http.StripPrefix("/docExtractor", mux).ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
	return securityHeaders(router)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"version": s.Version, "root": s.Organizer.Root(), "workers": s.Jobs.Workers()})
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	plans, err := s.Organizer.Scan()
	if err != nil {
		http.Error(w, "scan failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, plans)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Jobs.List()) }

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j, ok := s.Jobs.Get(r.PathValue("jobID"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, j)
}

func (s *Server) submitJobs(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var req submitRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(req.Names) == 0 || len(req.Names) > 500 {
		http.Error(w, "select 1 to 500 files", http.StatusBadRequest)
		return
	}
	submitted := make([]jobs.Job, 0, len(req.Names))
	for _, name := range req.Names {
		name = strings.TrimSpace(name)
		plan, err := s.Organizer.PlanName(name)
		if err != nil || plan.Error != "" || (plan.NeedsReview && !req.AllowReview) {
			continue
		}
		j, err := s.Jobs.Submit(jobs.Task{Source: plan.Source, Destination: plan.Destination, DeleteSource: true})
		if err != nil {
			if strings.Contains(err.Error(), "queue is full") {
				break
			}
			continue
		}
		submitted = append(submitted, j)
	}
	if len(submitted) == 0 {
		http.Error(w, "no executable files were submitted", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(submitted)
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	if !s.Jobs.Cancel(r.PathValue("jobID")) {
		http.Error(w, "job cannot be cancelled", http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

const indexHTML = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>docExtractor</title>
<style>body{font-family:system-ui,sans-serif;margin:0;background:#f5f6f8;color:#20242a}.wrap{max-width:1180px;margin:auto;padding:20px}header{display:flex;align-items:center;justify-content:space-between;gap:12px}.card{background:#fff;border:1px solid #dfe3e8;border-radius:10px;padding:16px;margin:14px 0}button{padding:9px 14px;border:1px solid #aeb6c1;border-radius:7px;background:#fff;cursor:pointer}.primary{background:#20242a;color:#fff}input[type=text]{min-width:280px;flex:1;padding:9px 10px;border:1px solid #aeb6c1;border-radius:7px;font:inherit}table{width:100%;border-collapse:collapse;font-size:14px}th,td{text-align:left;padding:8px;border-bottom:1px solid #e8ebef}.warn{color:#a15c00}.bad{color:#b42318}.ok{color:#087443}.muted{color:#69717d;font-size:13px}.actions{display:flex;gap:8px;flex-wrap:wrap;align-items:center}.scroll{overflow:auto}.pill{padding:2px 7px;border-radius:12px;background:#eef1f4}pre{white-space:pre-wrap;word-break:break-word;background:#111820;color:#e8eef5;padding:12px;border-radius:8px;max-height:360px;overflow:auto;font-size:12px}.hidden{display:none}a{color:#225ea8}.browser{margin-top:12px;border:1px solid #e0e4e8;border-radius:8px;padding:10px}.browser-list{display:flex;flex-direction:column;gap:4px;max-height:280px;overflow:auto;margin-top:8px}.browser-list button{text-align:left;width:100%}</style></head>
<body><div class="wrap"><header><div><h1>docExtractor</h1><div id="status" class="muted">loading...</div></div><button onclick="downloadDiag()">診断ZIP</button></header>
<section class="card"><strong>設定</strong><p class="muted">処理対象フォルダを保存します。QPKG再起動・アップデート後も維持されます。</p><div class="actions"><input id="rootInput" type="text" placeholder="/share/Download/Temp"><button class="primary" onclick="saveSettings()">保存</button><button onclick="openBrowser()">フォルダ選択</button></div><div id="settingsMsg" class="muted"></div><div id="browser" class="browser hidden"><div class="actions"><button id="browserUp" onclick="browseUp()">上へ</button><button onclick="chooseCurrent()">このフォルダを選択</button><button onclick="closeBrowser()">閉じる</button><span id="browserPath" class="muted"></span></div><div id="browserList" class="browser-list"></div></div></section>
<section class="card"><div class="actions"><button class="primary" onclick="scan()">スキャン</button><button onclick="runSelected(false)">安全な選択を実行</button><button onclick="runSelected(true)">確認対象も実行</button></div><p class="muted">ZIPは再圧縮せずrenameのみ。RARは中間展開せずストリーム変換します。</p><div class="scroll"><table><thead><tr><th></th><th>ファイル</th><th>シリーズ</th><th>巻</th><th>判定</th><th>処理</th></tr></thead><tbody id="plans"></tbody></table></div></section>
<section class="card"><button onclick="refreshJobs()">ジョブ更新</button><div class="scroll"><table><thead><tr><th>状態</th><th>ファイル</th><th>Stage</th><th>進捗</th><th>Read / Write</th><th>デバッグ</th></tr></thead><tbody id="jobs"></tbody></table></div></section>
<section id="logcard" class="card hidden"><div class="actions"><strong id="logtitle">ログ</strong><button onclick="closeLog()">閉じる</button></div><pre id="logview"></pre></section></div>
<script>
var APP_BASE=(location.pathname==='/'?'/':(location.pathname.endsWith('/')?location.pathname:location.pathname+'/'));var browserPath='';var browserParent='';
function appURL(p){return APP_BASE+String(p||'').replace(/^\/+/, '')}
function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot',"'":'&#39;'}[c]})}
function mb(n){return (Number(n||0)/1048576).toFixed(1)+' MB'}
async function api(path,opt){var r=await fetch(appURL(path),opt||{});if(!r.ok)throw new Error((await r.text()).trim());return r.status===204?null:r.json()}
async function refreshStatus(){var s=await api('api/status');document.getElementById('status').textContent=s.root+' / workers='+s.workers+' / '+s.version}
async function loadSettings(){var s=await api('api/settings');document.getElementById('rootInput').value=s.root||''}
async function saveSettings(){var msg=document.getElementById('settingsMsg');msg.className='muted';msg.textContent='保存中...';try{var saved=await api('api/settings',{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({root:document.getElementById('rootInput').value})});document.getElementById('rootInput').value=saved.root;msg.className='ok';msg.textContent='保存しました';await refreshStatus();await scan()}catch(e){msg.className='bad';msg.textContent='保存失敗: '+e.message}}
async function openBrowser(){document.getElementById('browser').classList.remove('hidden');var p=document.getElementById('rootInput').value;try{await browse(p)}catch(e){await browse('')}}
async function browse(p){var d=await api('api/directories'+(p?'?path='+encodeURIComponent(p):''));browserPath=d.path;browserParent=d.parent||'';document.getElementById('browserPath').textContent=d.path;document.getElementById('browserUp').disabled=!browserParent;var list=document.getElementById('browserList');list.innerHTML='';if(!d.entries.length){var empty=document.createElement('span');empty.className='muted';empty.textContent='サブフォルダなし';list.appendChild(empty);return}d.entries.forEach(function(e){var b=document.createElement('button');b.textContent='📁 '+e.name;b.onclick=function(){browse(e.path)};list.appendChild(b)})}
function browseUp(){if(browserParent)browse(browserParent)}
function chooseCurrent(){if(browserPath)document.getElementById('rootInput').value=browserPath;closeBrowser()}
function closeBrowser(){document.getElementById('browser').classList.add('hidden')}
async function init(){await refreshStatus();await loadSettings();await scan();await refreshJobs()}
async function scan(){var ps=await api('api/scan',{method:'POST'});var h='';ps.forEach(function(p){h+='<tr><td><input type="checkbox" data-name="'+esc(p.name)+'" '+(!p.needs_review&&!p.error?'checked':'')+' '+(p.error?'disabled':'')+'></td><td>'+esc(p.name)+(p.error?'<div class="bad">'+esc(p.error)+'</div>':'')+'</td><td>'+esc(p.series||'-')+'</td><td>'+(p.has_volume?esc(p.volume):'-')+'</td><td class="'+(p.needs_review?'warn':'ok')+'">'+Math.round((p.confidence||0)*100)+'% '+(p.needs_review?'確認':'OK')+'</td><td><span class="pill">'+esc(p.action||'-')+'</span></td></tr>'});document.getElementById('plans').innerHTML=h}
async function runSelected(allow){var names=Array.from(document.querySelectorAll('#plans input:checked')).map(function(x){return x.dataset.name});if(!names.length)return;await api('api/jobs',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({names:names,allow_review:allow})});await refreshJobs();await scan()}
async function refreshJobs(){var js=await api('api/jobs');var h='';js.forEach(function(j){var name=(j.task.source||'').split('/').pop();var id=encodeURIComponent(j.id);h+='<tr><td>'+esc(j.state)+'</td><td>'+esc(name)+'</td><td>'+esc(j.stage||'-')+'</td><td>'+Math.round((j.progress||0)*100)+'%</td><td>'+mb(j.bytes_read)+' / '+mb(j.bytes_written)+'</td><td><button onclick="showLog(\''+esc(j.id)+'\')">表示</button> · <a href="'+esc(appURL('api/logs/jobs/'+id+'/download'))+'">download</a> · <a href="'+esc(appURL('api/diagnostics/download?job_id='+id))+'">diagnostics</a>'+(j.state==='running'?' · <button onclick="cancelJob(\''+esc(j.id)+'\')">cancel</button>':'')+'</td></tr>'});document.getElementById('jobs').innerHTML=h;if(js.some(function(j){return j.state==='running'||j.state==='queued'}))setTimeout(refreshJobs,3000)}
async function showLog(id){var es=await api('api/logs/jobs/'+encodeURIComponent(id));var lines=es.map(function(e){return [e.time||'',String(e.level||'info').toUpperCase(),e.stage||'',e.message||'',e.error||'',e.bytes_read?('read='+mb(e.bytes_read)):'',e.bytes_written?('write='+mb(e.bytes_written)):''].filter(Boolean).join('  ')});document.getElementById('logtitle').textContent='ログ: '+id;document.getElementById('logview').textContent=lines.join('\n');document.getElementById('logcard').classList.remove('hidden');document.getElementById('logcard').scrollIntoView({behavior:'smooth',block:'start'})}
function closeLog(){document.getElementById('logcard').classList.add('hidden')}
async function cancelJob(id){await api('api/jobs/'+encodeURIComponent(id)+'/cancel',{method:'POST'});await refreshJobs()}
function downloadDiag(){location.href=appURL('api/diagnostics/download')}
init().catch(function(e){document.getElementById('status').textContent='Error: '+e.message})
</script></body></html>`
