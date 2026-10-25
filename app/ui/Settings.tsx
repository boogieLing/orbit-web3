"use client";

import { useState } from "react";
import { Check, LogOut, Save, Wallet } from "lucide-react";
import { api, countChars, errorText, mutation, shortAddress } from "./api";
import type { AppInfo, User } from "./api";
import { Busy, Modal, Notice } from "./shared";

// 管理昵称、主题偏好和钱包会话，逐项显示实际保存结果。
export default function Settings({ user, apps, onUser, onClose, onLogout, connectWallet, experience = false }: { user: User; apps: AppInfo[]; onUser: (user: User) => void; onClose: () => void; onLogout: () => Promise<void>; connectWallet: () => Promise<void>; experience?: boolean }) {
  const [name, setName] = useState(user.display_name);
  const [tags, setTags] = useState(user.preferences || []);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const choices = [...new Set([...apps.flatMap(app => app.tags), ...user.preferences])];
  // 锁定当前设置操作，统一管理错误和完成后的界面状态。
  const run = async (key: string, work: () => Promise<void>) => {
    setBusy(key); setError(""); setMessage("");
    try { await work(); } catch (reason) { setError(errorText(reason)); } finally { setBusy(""); }
  };
  return <Modal title="我的资料与阅读偏好" onClose={onClose} busy={!!busy}><div className="settings-content">
    <div className="profile-summary"><span className="profile-avatar">{user.display_name.slice(0, 1)}</span><div><strong>{user.display_name}</strong><span>{user.wallet ? shortAddress(user.wallet) : experience ? "社区身份 · 内容随当前浏览器会话保留" : "游客也可以收藏和创建精选页"}</span></div></div>
    <form onSubmit={event => { event.preventDefault(); void run("name", async () => { if (countChars(name.trim()) < 2 || countChars(name.trim()) > 30) throw new Error("昵称需要 2–30 个字。"); const result = await api<{ user: User }>("/profile", mutation("PATCH", { display_name: name.trim() })); onUser(result.user); setMessage("昵称已保存。"); }); }}><label>社区昵称<div className="inline-field"><input value={name} onChange={event => setName(event.target.value)} disabled={!!busy} /><button className="button secondary" disabled={!!busy || name.trim() === user.display_name}>{busy === "name" ? <Busy label="保存中" /> : "保存昵称"}</button></div></label></form>
    <div className="settings-section"><div className="eyebrow">MAKE IT YOURS</div><h3>最近对什么感兴趣？</h3><p>选择你想多看一些的主题，推荐会结合你的收藏与阅读反馈。</p><div className="interest-tags">{choices.map(tag => <button key={tag} disabled={!!busy} className={tags.includes(tag) ? "selected" : ""} aria-pressed={tags.includes(tag)} onClick={() => setTags(values => values.includes(tag) ? values.filter(value => value !== tag) : [...values, tag])}>{tags.includes(tag) && <Check size={13} />}{tag}</button>)}</div><button className="button primary" disabled={!!busy} onClick={() => void run("tags", async () => { const result = await api<{ user: User }>("/preferences", mutation("PUT", { tags })); onUser(result.user); setMessage("阅读偏好已保存。"); })}>{busy === "tags" ? <Busy label="保存中" /> : <><Save size={15} />保存阅读偏好</>}</button></div>
    {error && <Notice error>{error}</Notice>}{message && <div className="success-note" role="status"><Check size={15} />{message}</div>}
    {experience ? <div className="wallet-setting"><Check size={20} /><div><strong>社区身份已就绪</strong><p>直接发布体验、关联记录和创建精选页，无需连接钱包。</p></div></div> : <div className="wallet-setting"><Wallet size={20} /><div><strong>{user.wallet ? "钱包身份已连接" : "连接钱包，分享亲自用过的体验"}</strong><p>{user.wallet ? "签名用于社区登录，不会要求转账。" : "浏览、收藏和整理精选无需连接钱包。"}</p></div><button className="button quiet" disabled={!!busy} onClick={() => void run("wallet", async () => { if (user.wallet) { await onLogout(); onClose(); } else { await connectWallet(); setMessage("钱包身份已连接。"); } })}>{busy === "wallet" ? <Busy /> : user.wallet ? <><LogOut size={15} />退出</> : "连接"}</button></div>}
  </div></Modal>;
}
