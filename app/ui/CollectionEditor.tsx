"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Check, Code2, Copy, Download, Eye, LayoutGrid, Monitor, Plus, Save, Smartphone, WandSparkles } from "lucide-react";
import { api, APIError, countChars, dateLabel, errorText, mutation } from "./api";
import type { AppInfo, Collection, CollectionDetail, Post } from "./api";
import { AppMark, Busy, Modal, Notice, SourceTag } from "./shared";

type Draft = Pick<Collection, "title" | "description" | "instructions" | "layout" | "theme" | "app_ids" | "post_ids">;
const blank: Draft = { title: "", description: "", instructions: "", layout: "grid", theme: "ink", app_ids: [], post_ids: [] };
// 提取精选页的可编辑字段，复制数组以隔离已保存版本。
const fromCollection = (value: Collection): Draft => ({ title: value.title, description: value.description, instructions: value.instructions, layout: value.layout, theme: value.theme, app_ids: [...value.app_ids], post_ids: [...value.post_ids] });

// 在原始页面前加入限制策略，使预览文档先受网络隔离约束。
function previewDocument(html: string) {
  // 预览内容使用独立来源，并以最先出现的策略禁止连接外部服务。
  const policy = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'";
  return `<!doctype html><meta http-equiv="Content-Security-Policy" content="${policy}">` + html.replace(/<!doctype[^>]*>/i, "");
}

// 管理精选内容、历史版本、源码和隔离预览，并提交真实保存操作。
export default function CollectionEditor({ initial, apps, posts, freeform, onSaved, onClose, experience = false }: {
  initial: Collection | null; apps: AppInfo[]; posts: Post[]; freeform: boolean; experience?: boolean;
  onSaved: (collection: Collection) => void; onClose: () => void;
}) {
  const [current, setCurrent] = useState(initial);
  const [draft, setDraft] = useState<Draft>(initial ? fromCollection(initial) : blank);
  const [preview, setPreview] = useState(initial);
  const [versions, setVersions] = useState<CollectionDetail["versions"]>([]);
  const [busy, setBusy] = useState(initial ? "load" : "");
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [message, setMessage] = useState("");
  const [instruction, setInstruction] = useState("");
  const [width, setWidth] = useState<"desktop" | "mobile">("desktop");
  const [tab, setTab] = useState<"preview" | "source">("preview");
  const [closePrompt, setClosePrompt] = useState(false);
  const source = useMemo(() => preview?.html ? previewDocument(preview.html) : "", [preview]);
  const dirty = JSON.stringify(draft) !== JSON.stringify(current ? fromCollection(current) : blank);
  const historical = !!preview && preview.version !== current?.version;
  const postChoices = useMemo(() => [...new Map(posts.map(post => [post.id, post])).values()], [posts]);

  useEffect(() => {
    if (!initial) return;
    const controller = new AbortController();
    api<CollectionDetail>(`/collections/${initial.id}`, { signal: controller.signal }).then(result => {
      setCurrent(result.collection); setPreview(result.collection); setDraft(fromCollection(result.collection)); setVersions(result.versions);
    }).catch(reason => { if (!controller.signal.aborted) setError(errorText(reason)); }).finally(() => { if (!controller.signal.aborted) setBusy(""); });
    return () => controller.abort();
  }, [initial]);

  // 更新精选表单字段并清除上一次成功回执。
  const change = <K extends keyof Draft>(key: K, value: Draft[K]) => { setDraft(previous => ({ ...previous, [key]: value })); setMessage(""); };
  // 切换精选内容选择，同时限制每种内容最多十二项。
  const toggle = (key: "app_ids" | "post_ids", id: string) => {
    const values = draft[key];
    if (!values.includes(id) && values.length >= 12) { setError("每种内容最多选择 12 项，请先移除一项。"); return; }
    change(key, values.includes(id) ? values.filter(value => value !== id) : [...values, id]);
  };
  // 以服务端保存结果更新当前版本、表单和父级列表。
  const refresh = async (collection: Collection) => {
    setCurrent(collection); setPreview(collection); setDraft(fromCollection(collection)); onSaved(collection);
    setVersions(previous => [{ version: collection.version, created_at: collection.updated_at }, ...previous.filter(value => value.version !== collection.version)]);
    setConflict(false);
  };
  // 校验精选输入并提交新页面或带版本的修改，保留并发冲突提示。
  const save = async (event: React.FormEvent) => {
    event.preventDefault(); setError(""); setMessage("");
    const title = draft.title.trim();
    if (countChars(title) < 2 || countChars(title) > 80) { setError("精选页名称需要 2–80 个字。"); return; }
    if (countChars(draft.description) > 500 || countChars(draft.instructions) > 1000) { setError("简介最多 500 个字，编辑说明最多 1000 个字。"); return; }
    if (!draft.app_ids.length && !draft.post_ids.length) { setError("先选择至少一个应用或一篇内容，再创建精选页。"); return; }
    // 校验通过才锁定表单；修改请求携带当前版本，由服务端阻止过期覆盖。
    setBusy("save");
    try {
      const result = await api<{ collection: Collection }>(current ? `/collections/${current.id}` : "/collections", mutation(current ? "PATCH" : "POST", { ...draft, title, ...(current ? { version: current.version } : {}) }));
      await refresh(result.collection); setMessage(`第 ${result.collection.version} 版已保存，预览已更新。`);
    } catch (reason) { setError(errorText(reason)); setConflict(reason instanceof APIError && reason.status === 409); }
    finally { setBusy(""); }
  };
  // 明确放弃当前表单后重新读取最新精选版本。
  const loadLatest = async () => {
    if (!current) return;
    // 最新版本会替换表单和历史列表，失败时继续保留已有输入。
    setBusy("load"); setError("");
    try { const result = await api<CollectionDetail>(`/collections/${current.id}`); await refresh(result.collection); setVersions(result.versions); }
    catch (reason) { setError(errorText(reason)); } finally { setBusy(""); }
  };
  // 读取所选历史版本用于预览，不改写当前编辑表单。
  const selectVersion = async (version: string) => {
    if (!current) return;
    // 当前版本直接复用已保存内容，历史请求只更新预览对象。
    if (Number(version) === current.version) { setPreview(current); return; }
    setBusy("version"); setError("");
    try { const result = await api<{ collection: Collection }>(`/collections/${current.id}/versions/${version}`); setPreview(result.collection); }
    catch (reason) { setError(errorText(reason)); } finally { setBusy(""); }
  };
  // 保存基础内容后提交自由创作要求，成功后展示新版本。
  const compose = async () => {
    if (!current || !instruction.trim()) return;
    // 未保存的结构化修改先停留在表单，避免创作结果覆盖本地编辑。
    if (dirty) { setError("请先保存左侧内容，再提交创作要求。"); return; }
    if (countChars(instruction.trim()) < 2 || countChars(instruction) > 1000) { setError("创作要求需要 2–1000 个字。"); return; }
    setBusy("compose"); setError(""); setMessage("");
    try { const result = await api<{ collection: Collection }>(`/collections/${current.id}/compose`, mutation("POST", { instruction: instruction.trim() })); await refresh(result.collection); setInstruction(""); setMessage("新版本已保存，预览已更新。"); }
    catch (reason) { setError(errorText(reason)); } finally { setBusy(""); }
  };
  // 将当前预览的完整 HTML 复制到剪贴板，失败时保留源码入口。
  const copy = async () => {
    try { await navigator.clipboard.writeText(preview?.html || ""); setMessage("当前预览的完整 HTML 已复制。"); } catch { setError("浏览器没有允许复制，请在源码中选择内容后复制。"); }
  };
  return <Modal title={current ? "编辑我的精选" : "创建我的精选"} full busy={!!busy} onClose={() => dirty ? setClosePrompt(true) : onClose()}>
    <div className="collection-workspace">
      <form className="collection-form" onSubmit={save}>
        <div className="eyebrow">YOUR LITTLE CORNER</div><h3>把值得留下的，<br />整理成一页。</h3><p className="muted">选好内容，安排版式。让下一次查找更轻松。</p>
        <label>精选页名称<span>{countChars(draft.title)} / 80</span><input value={draft.title} onChange={event => change("title", event.target.value)} placeholder="例如：我的链上生活入门清单" disabled={!!busy} /></label>
        <label>一句简介<textarea rows={2} value={draft.description} onChange={event => change("description", event.target.value)} placeholder="这份精选，想留给谁？" disabled={!!busy} /><small>{countChars(draft.description)} / 500</small></label>
        <fieldset disabled={!!busy}><legend>选择版式</legend><div className="segmented"><button type="button" className={draft.layout === "editorial" ? "active" : ""} onClick={() => change("layout", "editorial")}><Eye size={15} />编辑手记</button><button type="button" className={draft.layout === "grid" ? "active" : ""} onClick={() => change("layout", "grid")}><LayoutGrid size={15} />清单网格</button></div></fieldset>
        <fieldset disabled={!!busy}><legend>页面色调</legend><div className="theme-choices">{[{ id: "sand", name: "暖沙" }, { id: "sage", name: "鼠尾草" }, { id: "ink", name: "墨色" }].map(item => <button key={item.id} type="button" aria-pressed={draft.theme === item.id} className={`theme-choice theme-${item.id} ${draft.theme === item.id ? "selected" : ""}`} onClick={() => change("theme", item.id as Draft["theme"])}><span>{draft.theme === item.id && <Check size={12} />}</span>{item.name}</button>)}</div></fieldset>
        <details className="selection-group" open><summary>挑选应用 <span>{draft.app_ids.length} / 12</span></summary><div className="selection-list">{apps.map(app => <label key={app.id} className="selection-item"><input type="checkbox" checked={draft.app_ids.includes(app.id)} disabled={!!busy} onChange={() => toggle("app_ids", app.id)} /><AppMark app={app} /><span><strong>{app.name}</strong><small>{app.summary}</small></span></label>)}</div></details>
        <details className="selection-group"><summary>加入收藏与阅读内容 <span>{draft.post_ids.length} / 12</span></summary><div className="selection-list">{postChoices.length ? postChoices.map(post => <label key={post.id} className="selection-item"><input type="checkbox" checked={draft.post_ids.includes(post.id)} disabled={!!busy} onChange={() => toggle("post_ids", post.id)} /><span><SourceTag post={post} /><strong>{post.title}</strong></span></label>) : <p className="muted">暂时没有可加入的内容。你可以先挑选应用，或回到社区收藏内容。</p>}</div></details>
        <label>编辑说明 <span>可选</span><textarea rows={3} value={draft.instructions} onChange={event => change("instructions", event.target.value)} placeholder="记录这份精选的取舍、顺序或阅读建议" disabled={!!busy} /><small>{countChars(draft.instructions)} / 1000</small></label>
        <div className="collection-save"><button type="submit" className="button primary" disabled={!!busy}>{busy === "save" ? <Busy label="正在保存" /> : <><Save size={16} />{current ? "保存新版本" : "创建并预览"}</>}</button><span>{dirty ? "有尚未保存的修改" : current ? `已保存 · 第 ${current.version} 版` : "无需连接钱包"}</span></div>
      </form>
      <div className="collection-preview-area">
        <div className="preview-toolbar"><div className="preview-tabs"><button className={tab === "preview" ? "active" : ""} onClick={() => setTab("preview")}><Eye size={15} />预览</button><button className={tab === "source" ? "active" : ""} onClick={() => setTab("source")}><Code2 size={15} />源码</button></div><div className="preview-controls"><button className={`icon-button ${width === "desktop" ? "selected" : ""}`} aria-label="桌面宽度" onClick={() => setWidth("desktop")}><Monitor size={16} /></button><button className={`icon-button ${width === "mobile" ? "selected" : ""}`} aria-label="手机宽度" onClick={() => setWidth("mobile")}><Smartphone size={16} /></button>{current && <select aria-label="查看版本" value={preview?.version || current.version} disabled={!!busy} onChange={event => void selectVersion(event.target.value)}>{(versions.length ? versions : [{ version: current.version, created_at: current.updated_at }]).map(version => <option key={version.version} value={version.version}>第 {version.version} 版 · {dateLabel(version.created_at)}</option>)}</select>}</div></div>
        {error && <Notice error>{error}{conflict && <button className="text-button" disabled={!!busy} onClick={() => void loadLatest()}>载入最新版本（会替换当前表单）</button>}</Notice>}
        {message && <div className="success-note" role="status"><Check size={16} />{message}</div>}
        {historical && <div className="history-note">正在查看第 {preview?.version} 版，左侧仍编辑当前版本。<button className="text-button" onClick={() => setPreview(current)}><ArrowLeft size={13} />返回当前版本</button></div>}
        <div className={`preview-canvas ${width === "mobile" ? "mobile-width" : ""}`}>
          {source ? tab === "preview" ? <iframe title={`${preview?.title || "精选页"}预览`} srcDoc={source} sandbox="allow-scripts" referrerPolicy="no-referrer" /> : <textarea className="source-view" readOnly value={preview?.html || ""} aria-label="精选页完整源码" spellCheck={false} /> : <div className="preview-empty"><span><LayoutGrid size={36} strokeWidth={1.2} /></span><h3>你的精选，从这里开始</h3><p>在左侧挑选内容并创建，<br />这里会显示可浏览的完整页面。</p></div>}
        </div>
        <div className="preview-export"><span>{preview ? `${preview.title} · 第 ${preview.version} 版` : "内容保存后即可复制或下载"}</span><div><button className="button quiet" disabled={!preview || !!busy} onClick={() => void copy()}><Copy size={15} />复制 HTML</button>{current && <a className="button quiet" href={`/api/v1/collections/${current.id}/export`} download><Download size={15} />下载当前版</a>}</div></div>
        <section className="compose-area"><div><WandSparkles size={18} /><h3>再多一点个人风格</h3></div>{freeform ? <><p>{experience ? "调整版式、配色、标题与简介，每次修改保存为新版本。" : "描述希望调整的排版和表达，将保留为新的版本。"}</p>{experience && <div className="compose-presets">{["深色卡片", "清爽列表", "暖色手记"].map(value => <button type="button" key={value} disabled={!!busy || !current || dirty} onClick={() => setInstruction(value)}>{value}</button>)}</div>}<textarea aria-label="页面调整要求" rows={2} value={instruction} onChange={event => setInstruction(event.target.value)} placeholder={experience ? "例如：深色卡片；标题：我的链上工具箱；简介：值得反复打开的应用和笔记" : "例如：让应用介绍更简洁，在开头加入阅读建议"} disabled={!!busy || !current} /><button className="button secondary" disabled={!!busy || !current || !instruction.trim() || dirty} onClick={() => void compose()}>{busy === "compose" ? <Busy label="正在构建新版本" /> : <><Plus size={15} />{experience ? "应用页面调整" : "应用创作要求"}</>}</button>{!current && <small>先创建精选页，即可继续调整。</small>}</> : <p>自由创作服务尚未配置。你仍可选择内容、版式和色调，保存完整的精选页。</p>}</section>
      </div>
    </div>
    {closePrompt && <div className="discard-panel" role="alert"><span>有尚未保存的修改。关闭后这些修改会被丢弃。</span><button className="button quiet" onClick={() => setClosePrompt(false)}>继续编辑</button><button className="button danger" onClick={onClose}>放弃修改并关闭</button></div>}
  </Modal>;
}
