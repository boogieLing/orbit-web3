"use client";

/* 复用已压缩的本地品牌素材，保持引导与首页一致。 */
/* eslint-disable @next/next/no-img-element */

import { useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, BookOpen, Check, Compass, FolderHeart, Wallet } from "lucide-react";
import type { AppInfo, User } from "./api";
import { api, errorText, mutation } from "./api";
import { Busy, Modal, Notice } from "./shared";

const storageKey = "usedit:onboarding:v1";
type Destination = "apps" | "discover";
type GuideProps = { user: User; apps: AppInfo[]; onUser: (user: User) => void; onFinish: (destination?: Destination) => void };

// 只读取引导完成标记；本地存储不可用时继续尝试当前会话存储。
function hasSeenGuide(): boolean {
  // 两种存储分别尝试，避免其中一种被禁用时丢失另一种完成标记。
  for (const storage of ["localStorage", "sessionStorage"] as const) {
    try { if (window[storage].getItem(storageKey) === "seen") return true; } catch { /* 存储限制不阻止首次访问。 */ }
  }
  return false;
}

// 记录用户已经完成或主动跳过，不存储兴趣、身份或其他个人内容。
function rememberGuide(): void {
  // 双写让刷新与同一会话中的返回都能记住用户的主动选择。
  for (const storage of ["localStorage", "sessionStorage"] as const) {
    try { window[storage].setItem(storageKey, "seen"); } catch { /* 无存储权限时仍允许正常进入产品。 */ }
  }
}

// 首次会话就绪后展示引导，手动入口递增请求值即可再次打开。
export default function Onboarding({ request, onStart, ...props }: Omit<GuideProps, "onFinish"> & { request: number; onStart: (destination: Destination) => void }) {
  // 父级仅在客户端会话就绪后挂载；重开通过组件键值重新初始化。
  const [open, setOpen] = useState(() => typeof window !== "undefined" && (request > 0 || !hasSeenGuide()));
  // 关闭与完成使用同一持久化出口，普通偏好刷新不会重新弹出引导。
  const finish = (destination?: Destination) => {
    rememberGuide(); setOpen(false);
    if (destination) onStart(destination);
  };
  return open ? <Guide {...props} onFinish={finish} /> : null;
}

// 将用途解释和真实兴趣保存连接起来，保存失败时保留选择供重试。
function Guide({ user, apps, onUser, onFinish }: GuideProps) {
  const [step, setStep] = useState(0);
  const [tags, setTags] = useState(user.preferences || []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const lock = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const initialStep = useRef(true);
  const choices = [...new Set([...apps.flatMap(app => app.tags), ...user.preferences])];

  // 步骤切换时把焦点交给新标题；首屏交由弹窗统一管理焦点。
  useEffect(() => {
    if (initialStep.current) { initialStep.current = false; return; }
    heading.current?.focus({ preventScroll: true });
    heading.current?.closest(".modal")?.scrollTo({ top: 0 });
  }, [step]);

  // 仅用户提交时写入已有偏好接口，确认成功后再退出并刷新推荐。
  const save = async () => {
    if (lock.current) return;
    lock.current = true; setBusy(true); setError("");
    try {
      // 服务端回执是完成依据，更新父级会话后才切换到推荐列表。
      const result = await api<{ user: User }>("/preferences", mutation("PUT", { tags }));
      onUser(result.user);
      onFinish("discover");
    } catch (reason) {
      setError(errorText(reason));
      lock.current = false; setBusy(false);
    }
  };

  return <Modal title="Orbit 引力 · 新手引导" onClose={() => onFinish()} busy={busy} className="onboarding-modal" wide>
    <div className="onboarding-body">
      <div className="onboarding-progress" aria-label={`第 ${step + 1} 步，共 2 步`}><span className="active" /><span className={step === 1 ? "active" : ""} /><small>0{step + 1} / 02</small></div>
      {step === 0 ? <div className="onboarding-intro">
        <div className="onboarding-copy"><div className="eyebrow">YOUR NEXT GOOD FIND</div><h3 ref={heading} tabIndex={-1}>找到下一款<br />值得用的 <span>Web3 应用。</span></h3><p className="onboarding-description">Orbit 引力，帮你发现应用、了解使用体验，<br className="onboarding-desktop-break" />再把好发现整理成自己的精选页。</p>
          <ol className="onboarding-benefits">
            <li><Compass size={19} /><div><strong>找应用</strong><span>从链上身份、交易工具、数字收藏开始。</span></div></li>
            <li><BookOpen size={19} /><div><strong>看体验</strong><span>阅读使用体验与资料笔记，看清来源再选择。</span></div></li>
            <li><FolderHeart size={19} /><div><strong>留住好发现</strong><span>收藏有用内容，整理成自己的精选页。</span></div></li>
          </ol>
        </div>
        <div className="onboarding-art" aria-hidden="true"><img src="/art/portal.webp" alt="" width={1536} height={1024} /><div><span>DISCOVER. TRY. KEEP.</span><strong>好发现，<br />自有引力。</strong></div></div>
      </div> : <div className="onboarding-preferences">
        <div className="eyebrow">MAKE IT YOURS</div><h3 ref={heading} tabIndex={-1}>你想先探索什么？</h3><p className="onboarding-description">选几个感兴趣的主题，帮你整理推荐内容。<br />以后可以在「阅读偏好」里随时调整。</p>
        <div className="onboarding-interests" role="group" aria-label="选择感兴趣的主题">{choices.map(tag => <button key={tag} type="button" aria-pressed={tags.includes(tag)} disabled={busy} onClick={() => setTags(values => values.includes(tag) ? values.filter(value => value !== tag) : [...values, tag])}><span className="interest-check">{tags.includes(tag) && <Check size={13} />}</span>{tag}</button>)}</div>
        <div className="onboarding-selection" aria-live="polite">{tags.length ? `已选 ${tags.length} 个主题` : "还没想好？可以直接进入应用目录。"}</div>
        <p className="onboarding-recommendation-note">推荐会参考兴趣、收藏和内容反馈；资料笔记与使用体验会分别标注。</p>
        {error && <Notice error>{error}</Notice>}
      </div>}
      <div className="onboarding-wallet-note"><Wallet size={15} /><span>{user.can_publish ? "浏览、发布、收藏、创建精选页，全部可以直接开始。" : "浏览、收藏、创建精选页，无需连接钱包。发布体验时再连接。"}</span></div>
    </div>
    <footer className="onboarding-footer">
      <div>{step === 1 ? <button className="onboarding-back" disabled={busy} onClick={() => { setStep(0); setError(""); }}><ArrowLeft size={15} />上一步</button> : <span>从一个好发现开始。</span>}</div>
      <div className="onboarding-actions"><button className="button quiet" disabled={busy} onClick={() => onFinish("apps")}>{step === 0 ? "跳过，先逛逛" : "暂不设置"}</button><button className="button primary" disabled={busy} onClick={() => { if (step === 0) setStep(1); else void save(); }}>{busy ? <Busy label="正在保存兴趣" /> : <>{step === 0 ? "选兴趣，开始探索" : "保存并查看推荐"}<ArrowRight size={17} /></>}</button></div>
    </footer>
  </Modal>;
}
