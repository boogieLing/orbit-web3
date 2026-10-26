"use client";
import {useEffect,useRef,useState} from 'react';
import {ArrowRight,Check,Code2,CornerDownLeft,Download,Eye,History,Layers,PenLine,X} from 'lucide-react';
import type {Run} from './types';

export const starterIdeas=[
 {label:'阅读收藏',kind:'个人工具',description:'收藏、搜索，整理值得再读的内容',text:'制作一个中文阅读收藏应用，可以添加文章标题、按标题搜索、删除条目，数据刷新后保留。采用简洁的深色界面和清晰的按钮。',icon:Layers},
 {label:'DApp 收藏夹',kind:'应用入口',description:'用标签整理常用的钱包与应用',text:'制作一个 DApp 收藏工具，可以添加名称、网址和标签，支持搜索、编辑、删除，数据刷新后保留。',icon:Code2},
 {label:'社区活动报名',kind:'社区协作',description:'查看活动、填写报名、管理名单',text:'制作一个社区活动报名工具，展示活动时间、地点和名额，支持填写昵称报名、取消和名单查询，数据刷新后保留。',icon:PenLine}
];
const steps=[{title:'描述需求',text:'说清楚谁来用、能做什么。也可以从一个例子开始。',icon:PenLine},{title:'运行与试用',text:'应用完成后直接操作，检查按钮、输入和数据。',icon:Eye},{title:'继续完善',text:'在同一项目提出修改，历史版本随时可以恢复。',icon:History}];

// StudioGuide 使用原生对话框提供焦点约束与 Escape 关闭；选择示例只填入需求。
export function StudioGuide({open,onClose,onStart,disabled=false}:{open:boolean;onClose:()=>void;onStart:()=>void;disabled?:boolean}){
 const dialog=useRef<HTMLDialogElement>(null);
 useEffect(()=>{const node=dialog.current;if(!node)return;if(open&&!node.open)node.showModal();if(!open&&node.open)node.close()},[open]);
 return <dialog className="studio-guide" ref={dialog} aria-labelledby="studio-guide-title" onCancel={onClose} onClose={onClose}>
  <button className="studio-guide-close" onClick={onClose} aria-label="关闭使用指引"><X size={20}/></button>
  <span className="studio-eyebrow">WELCOME TO ORBIT</span><h2 id="studio-guide-title">把一句想法，<br/>做成能用的应用。</h2><p className="studio-guide-lead">从阅读清单、收藏工具到活动报名。你描述需求，Orbit 完成代码与运行预览。</p>
  <ol className="studio-guide-steps">{steps.map((step,index)=><li key={step.title}><span>{String(index+1).padStart(2,'0')}</span><div><h3>{step.title}</h3><p>{step.text}</p></div><step.icon size={20}/></li>)}</ol>
  <div className="studio-guide-note"><Check size={16}/><span>项目与版本自动保存。无需安装软件或填写密钥。</span></div>
  <button className="studio-primary" autoFocus onClick={onStart} disabled={disabled}>用「阅读收藏」开始<ArrowRight size={17}/></button><button className="studio-guide-skip" onClick={onClose}>我想自己写需求</button><small>{disabled?'当前任务结束后，可以用示例开始新项目。':'选择示例只会填入需求，点击「开始制作」后才会运行。'}</small>
 </dialog>
}

// FlowSteps 用实际是否已有版本说明当前阶段，不把模型调用伪装成百分比进度。
export function FlowSteps({ready,active}:{ready:boolean;active:boolean}){
 return <ol className="studio-flow" aria-label="应用制作流程"><li data-complete={ready}><span>{ready?<Check size={12}/>:'01'}</span>描述需求</li><i/><li aria-current={active?'step':undefined} data-complete={ready}><span>{ready?<Check size={12}/>:'02'}</span>{active?'正在制作':'运行预览'}</li><i/><li aria-current={ready&&!active?'step':undefined}><span>03</span>继续完善与导出</li></ol>
}

// CreationStatus 只计时实际任务，卸载时清理定时器；不推测服务端细分阶段。
export function CreationStatus({run,hasVersion}:{run:Run;hasVersion:boolean}){
 const [now,setNow]=useState(()=>Date.now());
 useEffect(()=>{const timer=setInterval(()=>setNow(Date.now()),1000);return()=>clearInterval(timer)},[run.id]);
 const elapsed=Math.max(0,Math.floor((now-Date.parse(run.created_at))/1000));
 return <div className="studio-creation-status" role="status"><div className="studio-task-signal"/><div><strong>{run.status==='queued'?'需求已保存':'正在制作应用'}</strong><p>{hasVersion?'新版本完成后自动更新，已有版本仍然保留。':'完成后会自动显示应用，你可以直接试用。'}</p></div><time>{Number.isFinite(elapsed)?`${elapsed} 秒`:''}</time></div>
}

// PreviewPlaceholder 把等待状态与首次空态分开，明确完成后可做的操作。
export function PreviewPlaceholder({active}:{active:boolean}){
 return <section className="studio-preview-empty"><div className={`studio-window-outline ${active?'is-building':''}`} aria-hidden="true"><div><i/><i/><i/></div><span/><span/><span/><span/></div><span className="studio-eyebrow">YOUR APP, IN THIS SPACE</span><h2>{active?'正在把想法变成应用':'第一个版本，从这里开始'}</h2><p>{active?'制作完成后，预览会自动出现。你可以添加内容、点击按钮，再继续提出修改。':'在左侧描述你需要的功能。应用完成后，就能在这里直接使用。'}</p><div className="studio-preview-capabilities"><span><Eye size={15}/>运行预览</span><span><Code2 size={15}/>查看代码</span><span><Download size={15}/>独立导出</span></div></section>
}

// ComposerTip 按查看版本与任务状态给出下一步，禁用原因始终有文字说明。
export function ComposerTip({historical,active}:{historical:boolean;active:boolean}){
 return <span>{historical?'正在查看历史版本，请先恢复或返回当前版本':active?'当前任务完成后，可以继续提交修改':<><CornerDownLeft size={12}/> ⌘ / Ctrl + Enter 提交 · 草稿自动保留</>}</span>
}
