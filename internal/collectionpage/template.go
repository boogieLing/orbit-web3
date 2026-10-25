package collectionpage

const pageTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'none'">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · 用过再说</title>
<style>
:root{color-scheme:light;--bg:#f4f5ef;--surface:#fff;--ink:#26372d;--muted:#647265;--line:#dce3d8;--accent:#49624d;--soft:#e8eee4;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC",sans-serif}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);line-height:1.7}.theme-ink{--bg:#f1f2f3;--ink:#23272b;--muted:#697078;--line:#d9dde0;--accent:#343b42;--soft:#e6e9ec}.theme-sand{--bg:#f6f1e9;--ink:#493e31;--muted:#7c7062;--line:#e1d8ca;--accent:#806347;--soft:#eee3d3}
main{max-width:1140px;margin:0 auto;padding:52px 32px 72px}.brand{font-size:12px;letter-spacing:.12em;text-transform:uppercase;color:var(--muted)}header{padding:28px 0 34px;border-bottom:1px solid var(--line)}h1{font-size:clamp(28px,4vw,48px);line-height:1.25;letter-spacing:-.04em;margin:10px 0 18px;overflow-wrap:anywhere}header p{max-width:760px;white-space:pre-wrap;margin:0;color:var(--muted)}.counts{font-size:13px;color:var(--muted);margin-top:22px}
.filters{display:grid;grid-template-columns:2fr 1fr 1fr auto;gap:14px;align-items:end;padding:24px 0}label{display:grid;gap:6px;color:var(--muted);font-size:12px}input,select,button{font:inherit;color:var(--ink);border:1px solid var(--line);background:var(--surface);border-radius:8px;min-height:42px;padding:8px 12px;max-width:100%}input{width:100%}button{cursor:pointer;color:var(--accent)}button:hover{background:var(--soft)}button:active{transform:translateY(1px)}:focus-visible{outline:3px solid var(--accent);outline-offset:3px}.status{color:var(--muted);font-size:13px;margin:0 0 18px}
section{margin:30px 0}section h2{font-size:18px;letter-spacing:-.02em;margin:0 0 18px}.cards{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px}.layout-editorial .cards{grid-template-columns:1fr}.layout-editorial article{display:grid;grid-template-columns:220px minmax(0,1fr);gap:28px}.layout-editorial .card-body{padding-top:0}article{background:var(--surface);border:1px solid var(--line);border-radius:12px;padding:24px;min-width:0}article h3{font-size:20px;line-height:1.4;letter-spacing:-.02em;margin:10px 0;overflow-wrap:anywhere}.eyebrow{display:flex;gap:8px;flex-wrap:wrap;font-size:11px;color:var(--accent);align-items:center}.kind{background:var(--soft);border-radius:4px;padding:2px 7px}.card-body{white-space:pre-wrap;overflow-wrap:anywhere;color:var(--muted);font-size:14px}.tags{display:flex;flex-wrap:wrap;gap:7px;margin-top:16px}.tags span{font-size:11px;background:var(--bg);border:1px solid var(--line);padding:2px 8px;border-radius:20px}.empty{padding:40px 0;color:var(--muted)}footer{border-top:1px solid var(--line);padding-top:24px;margin-top:50px;color:var(--muted);font-size:12px}[hidden]{display:none!important}
@media(max-width:720px){main{padding:28px 18px 44px}.filters{grid-template-columns:1fr 1fr}.search-label{grid-column:1/-1}.cards{grid-template-columns:1fr}.layout-editorial article{display:block}.layout-editorial .card-body{padding-top:8px}article{padding:20px}header{padding-top:18px}.filters button{grid-column:1/-1}}
</style>
</head>
<body class="theme-{{.Theme}} layout-{{.Layout}}">
<main>
<div class="brand">UsedIt / 个人精选</div>
<header><h1>{{.Title}}</h1>{{if .Description}}<p>{{.Description}}</p>{{end}}<div class="counts">{{len .Apps}} 个应用 · {{len .Posts}} 篇记录</div></header>
<div class="filters" role="search" aria-label="筛选精选内容">
<label class="search-label">搜索内容<input id="search" type="search" placeholder="搜索名称、正文或标签" maxlength="200" autocomplete="off"></label>
<label>内容类型<select id="kind"><option value="all">全部内容</option><option value="应用">应用</option><option value="使用体验">使用体验</option><option value="资料笔记">资料笔记</option></select></label>
<label>应用分类<select id="category"><option value="all">全部分类</option>{{range .Categories}}<option value="{{.}}">{{.}}</option>{{end}}</select></label>
<button id="reset" type="button">清除筛选</button>
</div>
<p id="status" class="status" role="status" aria-live="polite"></p>
{{if .Apps}}<section data-section><h2>应用精选</h2><div class="cards">
{{range .Apps}}<article data-card data-kind="应用" data-category="{{.Category}}" data-search="{{.Search}}"><div><div class="eyebrow"><span class="kind">应用</span><span>{{.Category}}</span></div><h3>{{.Title}}</h3></div><div><div class="card-body">{{.Body}}</div>{{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.}}</span>{{end}}</div>{{end}}</div></article>{{end}}
</div></section>{{end}}
{{if .Posts}}<section data-section><h2>阅读记录</h2><div class="cards">
{{range .Posts}}<article data-card data-kind="{{.Label}}" data-category="{{.Category}}" data-search="{{.Search}}"><div><div class="eyebrow"><span class="kind">{{.Label}}</span>{{if .AppName}}<span>{{.AppName}}</span>{{end}}{{if .Sentiment}}<span>{{.Sentiment}}</span>{{end}}</div><h3>{{.Title}}</h3></div><div><div class="card-body">{{.Body}}</div>{{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.}}</span>{{end}}</div>{{end}}</div></article>{{end}}
</div></section>{{end}}
<p id="empty" class="empty" hidden>没有符合条件的内容。换个关键词，或清除筛选。</p>
<footer>用过再说 · 精选内容独立保存，资料笔记与使用体验分别标注。</footer>
</main>
<script>
(()=>{'use strict';
const search=document.getElementById('search'),kind=document.getElementById('kind'),category=document.getElementById('category'),cards=Array.from(document.querySelectorAll('[data-card]')),sections=Array.from(document.querySelectorAll('[data-section]'));
function filter(){const query=search.value.trim().toLocaleLowerCase();let count=0;cards.forEach(card=>{const visible=(!query||card.dataset.search.includes(query))&&(kind.value==='all'||card.dataset.kind===kind.value)&&(category.value==='all'||card.dataset.category===category.value);card.hidden=!visible;if(visible)count++;});sections.forEach(section=>{section.hidden=!Array.from(section.querySelectorAll('[data-card]')).some(card=>!card.hidden);});document.getElementById('status').textContent='显示 '+count+' / '+cards.length+' 项内容';document.getElementById('empty').hidden=count!==0;}
search.addEventListener('input',filter);kind.addEventListener('change',filter);category.addEventListener('change',filter);document.getElementById('reset').addEventListener('click',()=>{search.value='';kind.value='all';category.value='all';filter();search.focus();});filter();
})();
</script>
</body>
</html>`
