"use client";

import { useRef, useState } from "react";
import { ArrowRight, Link2, Send, Wallet, Check } from "lucide-react";
import type { AppInfo, Post, User } from "./api";
import { api, APIError, countChars, errorText, mutation } from "./api";
import { Busy, Modal, Notice } from "./shared";

export type PostDraft = { app_id: string; title: string; body: string; sentiment: Post["sentiment"]; tags: string; tx_hash: string };
// 创建空白体验草稿，供首次编辑和成功发布后重置使用。
export const blankDraft = (): PostDraft => ({ app_id: "", title: "", body: "", sentiment: "neutral", tags: "", tx_hash: "" });

// 编辑体验并连接签名发布流程，失败或取消时保留父级草稿。
export default function PostEditor({ apps, user, draft, onDraft, initial, onClose, onSaved, connectWallet }: {
  apps: AppInfo[]; user: User; draft: PostDraft; onDraft: (value: PostDraft) => void; initial?: Post | null;
  onClose: () => void; onSaved: (post: Post, note: string) => void; connectWallet: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState(false);
  const [revision, setRevision] = useState(initial?.revision);
  const request = useRef<{ body: string; key: string } | null>(null);
  // 将单个表单字段同步至父级草稿，保持关闭弹窗后的内容。
  const update = <K extends keyof PostDraft>(key: K, value: PostDraft[K]) => onDraft({ ...draft, [key]: value });
  const app = apps.find(item => item.id === draft.app_id);
  const tags = [...new Set(draft.tags.split(/[,，]/).map(tag => tag.trim()).filter(Boolean))];
  // 校验体验输入，以幂等键发布或检查版本保存，并单独核对凭据。
  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setError(""); setBusy(true);
    try {
      if (!user.wallet) { await connectWallet(); return; }
      if (!draft.app_id) throw new Error("先选择你使用过的应用。");
      if (countChars(draft.title.trim()) < 2 || countChars(draft.title.trim()) > 120) throw new Error("标题需要 2 至 120 个字符。");
      if (countChars(draft.body.trim()) < 20 || countChars(draft.body.trim()) > 5000) throw new Error("请用 20 至 5000 个字符描述具体使用过程。");
      if (tags.length > 5 || tags.some(tag => countChars(tag) > 20)) throw new Error("最多添加 5 个标签，每个标签不超过 20 个字符。");
      if (draft.tx_hash.trim() && !/^0x[0-9a-f]{64}$/i.test(draft.tx_hash.trim())) throw new Error("交易哈希应为 0x 开头的 66 位十六进制字符串。");
      const body = { title: draft.title.trim(), body: draft.body.trim(), sentiment: draft.sentiment, tags, ...(initial ? { revision } : { app_id: draft.app_id, ...(draft.tx_hash.trim() ? { tx_hash: draft.tx_hash.trim() } : {}) }) };
      const encoded = JSON.stringify(body);
      // 内容未改变时复用提交标识，让网络重试不会重复创建体验。
      if (request.current?.body !== encoded) request.current = { body: encoded, key: crypto.randomUUID() };
      const result = await api<{ post: Post }>(initial ? `/posts/${initial.id}` : "/posts", mutation(initial ? "PATCH" : "POST", body, initial ? undefined : { "Idempotency-Key": request.current.key }));
      let post = result.post;
      let note = initial ? "修改已保存" : "体验已发布";
      // 凭据核对独立于发布结果，附属请求失败也保留已成功保存的体验。
      if (!initial && draft.tx_hash.trim()) {
        try {
          const verified = await api<{ post: Post }>(`/posts/${post.id}/verify`, mutation("POST", { tx_hash: draft.tx_hash.trim() }));
          post = verified.post; note += post.proof_status === "verified" ? "，交互凭据已核对" : "，凭据状态可在详情查看";
        } catch (proofError) { note += `；凭据尚未核对：${errorText(proofError)}`; }
      }
      onSaved(post, note);
    } catch (submitError) { setError(errorText(submitError)); setConflict(submitError instanceof APIError && submitError.status === 409); }
    finally { setBusy(false); }
  };
  // 在明确替换提示下读取最新内容，解除作者编辑时的版本冲突。
  const reloadCurrent = async () => {
    if (!initial) return;
    setBusy(true); setError("");
    try {
      const result = await api<{ post: Post }>(`/posts/${initial.id}`);
      const post = result.post;
      // 正文和修订号同时替换，保证下一次保存基于刚读取的同一版本。
      setRevision(post.revision); onDraft({ app_id: post.app_id, title: post.title, body: post.body, sentiment: post.sentiment, tags: post.tags.join("，"), tx_hash: post.tx_hash || "" }); setConflict(false);
    } catch (reason) { setError(errorText(reason)); } finally { setBusy(false); }
  };
  return <Modal title={initial ? "编辑使用体验" : "用过了，就来聊聊"} onClose={onClose} busy={busy} wide>
    <form className="post-form" onSubmit={event => void submit(event)}>
      <div className="form-intro"><span className="tiny-label">YOUR EXPERIENCE MATTERS</span><h3>好用的地方，<br />不顺手的地方，都值得记下来。</h3><p>具体的使用场景，比一句“不错”更有帮助。</p></div>
      {error && <Notice error>{error}{conflict && initial && <button className="text-button" type="button" disabled={busy} onClick={() => void reloadCurrent()}>载入最新内容（会替换当前草稿）</button>}</Notice>}
      <label className="field">体验的是哪个应用<select value={draft.app_id} onChange={event => update("app_id", event.target.value)} disabled={busy || !!initial}><option value="">选择一个应用</option>{apps.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="field">一句话说说感受<input value={draft.title} onChange={event => update("title", event.target.value)} placeholder="例如：第一次设置链上身份，哪些步骤值得注意" maxLength={240} disabled={busy} /><small>{countChars(draft.title)} / 120</small></label>
      <label className="field">具体使用体验<textarea value={draft.body} onChange={event => update("body", event.target.value)} placeholder="你想解决什么问题？实际用了哪些功能？哪里符合预期，哪里还可以改进？" rows={7} maxLength={10000} disabled={busy} /><small>至少 20 字 · {countChars(draft.body)} / 5000</small></label>
      <fieldset className="choice-field"><legend>你的整体感受</legend><div className="sentiment-options">{[{ value: "positive", label: "值得尝试" }, { value: "neutral", label: "各有取舍" }, { value: "negative", label: "还有不足" }].map(item => <button key={item.value} type="button" className={draft.sentiment === item.value ? "selected" : ""} onClick={() => update("sentiment", item.value as Post["sentiment"])} disabled={busy}>{draft.sentiment === item.value && <Check size={14} />}{item.label}</button>)}</div></fieldset>
      <label className="field">添加标签<span className="optional">可选</span><input value={draft.tags} onChange={event => update("tags", event.target.value)} placeholder="用逗号分隔，最多 5 个标签" disabled={busy} /></label>
      {!!app?.tags.length && <div className="tag-suggestions">{app.tags.slice(0, 5).map(tag => <button type="button" key={tag} disabled={busy || tags.includes(tag) || tags.length >= 5} onClick={() => update("tags", [...tags, tag].join("，"))}>+ {tag}</button>)}</div>}
      {!initial && <details className="proof-disclosure"><summary><Link2 size={16} /><span>附上交互凭据</span><small>可选</small><ArrowRight size={14} /></summary><p>填写当前钱包在 Ethereum 主网与该应用交互的交易哈希。核对结果只表明这次交互发生过。</p><label className="field">交易哈希<input className="mono" value={draft.tx_hash} onChange={event => update("tx_hash", event.target.value.trim())} placeholder="0x…" maxLength={66} disabled={busy} /></label></details>}
      {!user.wallet && <div className="wallet-notice"><Wallet size={20} /><div><strong>连接钱包后发布</strong><p>签名仅用于验证身份。取消连接或签名不会清空草稿。</p></div></div>}
      <div className="modal-actions"><span className="muted">{initial ? "保存时会检查版本，避免覆盖其他页面的修改。" : "离开弹窗后，本次草稿仍会保留。"}</span><button className="primary-button" type="submit" disabled={busy}>{busy ? <Busy label={user.wallet ? "正在保存" : "等待钱包"} /> : user.wallet ? <>{initial ? "保存修改" : "发布体验"}<Send size={15} /></> : <>连接钱包<Wallet size={16} /></>}</button></div>
    </form>
  </Modal>;
}
