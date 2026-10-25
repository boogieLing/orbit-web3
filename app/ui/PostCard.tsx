"use client";

import { useEffect, useRef, useState } from "react";
import { Bookmark, ThumbsUp, EyeOff, ArrowUpRight, Pencil, Trash2, Link2, Check, Copy } from "lucide-react";
import type { AppInfo, Post } from "./api";
import { api, dateLabel, errorText, mutation, shortAddress } from "./api";
import { AppMark, Busy, ExternalLink, Modal, Notice, SourceTag, categoryName } from "./shared";

type Actions = { onOpen: (post: Post) => void; onBookmark: (post: Post) => void; onHelpful: (post: Post) => void; onHide: (post: Post) => void; pending: string | null };

// 展示真实内容摘要和反馈入口，进入可见区域后记录阅读曝光。
export default function PostCard({ post, app, onOpen, onBookmark, onHelpful, onHide, pending }: { post: Post; app?: AppInfo } & Actions) {
  const root = useRef<HTMLElement>(null);
  useEffect(() => {
    const element = root.current;
    if (!element || typeof IntersectionObserver === "undefined") return;
    let sent = false;
    const observer = new IntersectionObserver(entries => {
      if (sent || !entries.some(entry => entry.isIntersecting)) return;
      sent = true; observer.disconnect();
      void api("/feedback", mutation("POST", { post_id: post.id, kind: "impression", event_id: crypto.randomUUID() })).catch(() => { console.warn("曝光反馈暂未保存"); });
    }, { threshold: 0.6 });
    observer.observe(element);
    return () => observer.disconnect();
  }, [post.id]);
  return <article ref={root} className={`post-card ${post.kind === "reference" ? "reference" : ""}`}>
    <div className="post-card-top"><div className="app-line"><AppMark app={app} /><span><strong>{app?.name || "应用体验"}</strong><small>{app ? categoryName(app.category) : "社区记录"}</small></span></div><SourceTag post={post} /></div>
    <button className="post-open" onClick={() => onOpen(post)}><h3>{post.title}<ArrowUpRight size={18} /></h3><p>{post.body}</p></button>
    {!!post.tags?.length && <div className="post-tags">{post.tags.slice(0, 4).map(tag => <span key={tag}>{tag}</span>)}</div>}
    <div className="post-byline"><span className={`author-mark ${post.kind === "reference" ? "editor" : ""}`}>{post.kind === "reference" ? <Pencil size={12} /> : (post.author.name || "读者").slice(0, 1)}</span><span>{post.kind === "reference" ? "官方资料整理" : post.author.name || shortAddress(post.author.wallet)}</span><span className="byline-separator">·</span><time>{dateLabel(post.created_at)}</time>{post.kind === "reference" && <span className="reference-note">非使用评价</span>}</div>
    <div className="post-bottom"><div className="post-actions"><button aria-label={post.helpful ? "取消有用" : "标记有用"} aria-pressed={post.helpful} className={post.helpful ? "active" : ""} disabled={!!pending} onClick={() => onHelpful(post)}><ThumbsUp size={15} />有用{post.helpful_count > 0 && <span>{post.helpful_count}</span>}</button><button aria-label={post.bookmarked ? "取消收藏" : "收藏这条内容"} aria-pressed={post.bookmarked} className={post.bookmarked ? "active" : ""} disabled={!!pending} onClick={() => onBookmark(post)}><Bookmark size={15} fill={post.bookmarked ? "currentColor" : "none"} />{post.bookmarked ? "已收藏" : "收藏"}</button></div><button className="hide-button" aria-label="不感兴趣" title="不感兴趣" onClick={() => onHide(post)} disabled={!!pending}><EyeOff size={15} /></button></div>
    {post.reason && <div className="recommendation-reason"><span />{post.reason}</div>}
  </article>;
}

// 展示完整内容、资料来源和凭据结果，并管理作者操作。
export function PostDetail({ post, app, onClose, onBookmark, onHelpful, onEdit, onDelete, onUpdate, pending }: {
  post: Post; app?: AppInfo; onClose: () => void; onBookmark: (post: Post) => void; onHelpful: (post: Post) => void;
  onEdit: (post: Post) => void; onDelete: (post: Post) => Promise<void>; onUpdate: (post: Post) => void; pending: string | null;
}) {
  const [hash, setHash] = useState(post.tx_hash || "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [copied, setCopied] = useState(false);
  // 按当前交易哈希手动请求核对，保留服务端返回的完整状态。
  const verify = async () => {
    if (!/^0x[0-9a-f]{64}$/i.test(hash)) { setError("请填写完整的 Ethereum 交易哈希。"); return; }
    // 每次核对只发出一次请求；失败保留哈希，供作者自行决定重试。
    setBusy(true); setError("");
    try { const result = await api<{ post: Post }>(`/posts/${post.id}/verify`, mutation("POST", { tx_hash: hash })); onUpdate(result.post); }
    catch (verifyError) { setError(errorText(verifyError)); }
    finally { setBusy(false); }
  };
  // 删除作者自己的体验，失败时保留详情与错误说明。
  const remove = async () => { setBusy(true); setError(""); try { await onDelete(post); } catch (deleteError) { setError(errorText(deleteError)); setBusy(false); } };
  const statuses = { none: "尚未附加交互凭据", pending: "交易等待进一步确认", verified: "交互凭据已核对", unavailable: "数据源暂时无法查询", rejected: "这笔交易未通过核对" };
  return <Modal title={post.kind === "reference" ? "资料笔记" : "使用体验"} onClose={onClose} wide busy={busy}>
    <article className="detail-article"><div className="detail-app"><AppMark app={app} large /><div><strong>{app?.name || "应用"}</strong><span>{app ? categoryName(app.category) : "体验记录"}</span></div><SourceTag post={post} /></div><h1>{post.title}</h1><div className="detail-byline"><span>{post.kind === "reference" ? "官方资料整理" : post.author.name}</span>{post.author.wallet && post.kind !== "reference" && <code>{shortAddress(post.author.wallet)}</code>}<span>{dateLabel(post.created_at)}</span></div>
      {post.kind === "reference" && <div className="reference-disclaimer"><Pencil size={15} /><p>这是一则带来源的资料笔记，用于了解应用，不代表任何用户的实际使用体验。</p></div>}
      <div className="article-body">{post.body.split(/\n\n+/).map((paragraph, index) => <p key={index}>{paragraph}</p>)}</div><div className="post-tags">{post.tags.map(tag => <span key={tag}>{tag}</span>)}</div>
      {post.kind === "reference" ? <div className="source-box"><BookSource /><span>原始资料</span><ExternalLink href={post.source_url || app?.source_url}>阅读来源</ExternalLink></div> : <section className="proof-box"><div className="proof-box-heading"><Link2 size={18} /><strong>{statuses[post.proof_status]}</strong>{post.proof_status === "verified" && <Check size={18} />}</div><p>{post.proof_message || "可核对交易发送者、目标合约、执行结果与确认深度。"}</p><small>仅说明钱包与应用的这次交互，不评价应用安全或内容质量。</small>{post.tx_hash && /^0x[0-9a-f]{64}$/i.test(post.tx_hash) && <div className="hash-row"><ExternalLink href={`https://etherscan.io/tx/${post.tx_hash}`}>{shortAddress(post.tx_hash)}</ExternalLink><button className="icon-button" aria-label="复制交易哈希" onClick={() => void navigator.clipboard.writeText(post.tx_hash).then(() => setCopied(true)).catch(() => setError("复制未完成，请手动选择交易哈希。"))}>{copied ? <Check size={14} /> : <Copy size={14} />}</button></div>}{post.owned && <div className="proof-input"><label htmlFor="proof-hash">交易哈希</label><div><input id="proof-hash" className="mono" value={hash} onChange={event => setHash(event.target.value.trim())} placeholder="0x…" maxLength={66} disabled={busy} /><button className="secondary-button" onClick={() => void verify()} disabled={busy || !hash}>{busy ? <Busy /> : post.tx_hash ? "重新核对" : "核对凭据"}</button></div></div>}</section>}
      {error && <Notice error>{error}</Notice>}
      <div className="detail-footer"><div className="post-actions"><button className={post.helpful ? "active" : ""} disabled={!!pending} onClick={() => onHelpful(post)}><ThumbsUp size={17} />{post.helpful ? "已标有用" : "对我有用"}{post.helpful_count > 0 && <span>{post.helpful_count}</span>}</button><button className={post.bookmarked ? "active" : ""} disabled={!!pending} onClick={() => onBookmark(post)}><Bookmark size={17} fill={post.bookmarked ? "currentColor" : "none"} />{post.bookmarked ? "已收藏" : "收藏"}</button></div><ExternalLink className="text-link" href={app?.url}>访问应用</ExternalLink></div>
      {post.owned && <div className="owner-actions"><button onClick={() => onEdit(post)} disabled={busy}><Pencil size={14} />编辑体验</button><button className="danger-link" onClick={() => setConfirmDelete(value => !value)} disabled={busy}><Trash2 size={14} />删除</button></div>}{confirmDelete && <div className="delete-confirm"><p>删除后这条体验将不再出现在社区。</p><button onClick={() => setConfirmDelete(false)} disabled={busy}>保留</button><button className="danger-button" disabled={busy} onClick={() => void remove()}>{busy ? <Busy /> : "确认删除"}</button></div>}
    </article>
  </Modal>;
}

// 展示来源链接的小型方向图标。
function BookSource() { return <ArrowUpRight size={17} />; }
