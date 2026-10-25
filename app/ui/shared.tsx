"use client";

import { useEffect, useId, useRef } from "react";
import type { ReactNode } from "react";
import { ArrowDownUp, ArrowUpRight, BookOpen, Orbit, Check, Fingerprint, Shapes, X, LoaderCircle, CircleAlert, Link2 } from "lucide-react";
import type { AppInfo, Post } from "./api";
import { externalHref } from "./api";

export const categories = [{ id: "all", label: "全部应用", Icon: Shapes }, { id: "identity", label: "链上身份", Icon: Fingerprint }, { id: "exchange", label: "交易工具", Icon: ArrowDownUp }, { id: "collectibles", label: "数字收藏", Icon: BookOpen }];
// 将应用分类标识映射为界面中的中文名称。
export const categoryName = (id: string) => categories.find(category => category.id === id)?.label || "应用";

// 展示社区品牌文字与图形标记，供页头页脚复用。
export function Logo() { return <span className="brand"><span className="brand-symbol"><Orbit size={24} strokeWidth={1.7} aria-hidden="true" /></span><span>ORBIT<small>引力 · WEB3</small></span></span>; }

// 按应用分类展示统一图标，避免把装饰图形当成官方标志。
export function AppMark({ app, large = false }: { app?: AppInfo; large?: boolean }) {
  const category = categories.find(item => item.id === app?.category);
  const Icon = category?.Icon || Shapes;
  return <span className={`app-mark ${large ? "large" : ""} app-${app?.category || "all"}`}><Icon size={large ? 28 : 19} strokeWidth={1.7} /></span>;
}

// 根据真实内容类型和凭据状态展示不同的出处标签。
export function SourceTag({ post }: { post: Post }) {
  if (post.kind === "reference") return <span className="source-tag"><BookOpen size={12} />资料笔记</span>;
  if (post.proof_status === "recorded") return <span className="proof-tag"><Link2 size={12} />交互记录</span>;
  if (post.proof_status === "verified") return <span className="proof-tag"><Link2 size={12} />交互已核对</span>;
  return <span className="review-tag">使用体验</span>;
}

// 校验链接协议后开启外部页面，并隔离新页面的访问上下文。
export function ExternalLink({ href, children, className = "" }: { href?: string; children: ReactNode; className?: string }) {
  const safe = externalHref(href);
  return safe ? <a href={safe} className={className} target="_blank" rel="noopener noreferrer">{children}<ArrowUpRight size={14} /></a> : <span className={className}>{children}</span>;
}

// 提供键盘焦点约束、关闭控制和滚动锁定的可访问弹窗。
export function Modal({ title, children, onClose, wide = false, full = false, busy = false, className = "" }: { title: string; children: ReactNode; onClose: () => void; wide?: boolean; full?: boolean; busy?: boolean; className?: string }) {
  const titleID = useId();
  const root = useRef<HTMLDivElement>(null);
  const close = useRef(onClose);
  const locked = useRef(busy);
  useEffect(() => { close.current = onClose; locked.current = busy; }, [onClose, busy]);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const oldOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    root.current?.querySelector<HTMLElement>("button, input, textarea, select, [tabindex='0']")?.focus();
    // 处理关闭键和首尾焦点循环，让键盘操作留在当前弹窗。
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !locked.current) close.current();
      if (event.key !== "Tab") return;
      const controls = root.current?.querySelectorAll<HTMLElement>("button:not(:disabled),a[href],input:not(:disabled),textarea:not(:disabled),select:not(:disabled),[tabindex='0']");
      if (!controls?.length) return;
      const first = controls[0], last = controls[controls.length - 1];
      // 在首尾元素之间循环焦点，避免键盘进入被遮挡的背景页面。
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", keydown);
    return () => { document.body.style.overflow = oldOverflow; document.removeEventListener("keydown", keydown); previous?.focus(); };
  }, []);
  return <div className="modal-backdrop" onMouseDown={event => { if (event.target === event.currentTarget && !busy) onClose(); }}><section ref={root} className={`modal ${wide ? "wide" : ""} ${full ? "full" : ""} ${className}`} role="dialog" aria-modal="true" aria-labelledby={titleID}><header className="modal-header"><h2 id={titleID}>{title}</h2><button className="icon-button" aria-label="关闭弹窗" disabled={busy} onClick={onClose}><X size={20} /></button></header>{children}</section></div>;
}

// 以状态或错误语义展示可读反馈，支持辅助阅读工具。
export function Notice({ children, error = false }: { children: ReactNode; error?: boolean }) { return <div className={`notice ${error ? "error" : ""}`} role={error ? "alert" : "status"}><CircleAlert size={16} /><span>{children}</span></div>; }
// 展示真实请求进行中的状态，不使用推测的完成比例。
export function Busy({ label = "正在处理" }: { label?: string }) { return <span className="busy-label"><LoaderCircle size={15} className="spin" />{label}</span>; }
// 展示没有内容时的解释和下一步入口。
export function Empty({ title, children, action }: { title: string; children: ReactNode; action?: ReactNode }) { return <div className="empty-state"><span className="empty-icon"><BookOpen size={29} strokeWidth={1.4} /></span><h3>{title}</h3><p>{children}</p>{action}</div>; }
// 展示操作回执，并提供关闭或撤销等后续动作。
export function Toast({ message, onClose, action }: { message: string; onClose: () => void; action?: { label: string; run: () => void } }) { return <div className="toast" role="status"><Check size={17} /><span>{message}</span>{action && <button onClick={action.run}>{action.label}</button>}<button className="icon-button" aria-label="关闭提示" onClick={onClose}><X size={15} /></button></div>; }
