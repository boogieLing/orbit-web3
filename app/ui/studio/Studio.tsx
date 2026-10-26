"use client";
import {useEffect,useRef,useState} from 'react';
import {ArrowLeft,ArrowRight,ArrowUp,Check,ChevronRight,Code2,Folder,HelpCircle,History,Plus,RotateCcw} from 'lucide-react';
import {Logo} from '../shared';
import {Preview} from './Preview';
import {ComposerTip,CreationStatus,FlowSteps,PreviewPlaceholder,starterIdeas,StudioGuide} from './Guide';
import {Welcome} from './Welcome';
import {useStudio} from './useStudio';
import './studio.css';

// Studio 负责工作台层级与引导，持久化请求统一交给控制器。
export default function Studio(){
 const studio=useStudio();
 const {session,config,projects,detail,project,active,selectedRun,prompt,setPrompt,error,busy,booting,history,setHistory,version,messages,preview,openProject,newProject,submit,restore,selectVersion,resume}=studio;
 const [guide,setGuide]=useState(false);const guideChecked=useRef(false);
 const historical=!!project&&version!==project.version;
 // 仅首次空工作台自动介绍；关闭和 Escape 都记住选择，头部入口可重新打开。
 useEffect(()=>{if(booting||!session||guideChecked.current)return;guideChecked.current=true;try{if(projects.length===0&&!localStorage.getItem('orbit:studio:intro:v2'))setGuide(true)}catch{/* 存储受限时仍允许直接使用工作台。 */}},[booting,session,projects.length]);
 // 关闭后的偏好只影响引导展示，不改变项目。
 function closeGuide(){setGuide(false);try{localStorage.setItem('orbit:studio:intro:v2','seen')}catch{/* 引导偏好不影响创作保存。 */}}
 // 已有项目里打开帮助时不丢弃当前内容；示例只应用于新建入口。
 async function startExample(){if(busy||active)return;if(!await newProject())return;closeGuide();studio.setTitle(starterIdeas[0].label);setPrompt(starterIdeas[0].text)}
 return <div className="orbit-studio"><header className="studio-header"><a href="/" aria-label="返回 Orbit 社区"><Logo/></a><span className="studio-header-divider"/><span>STUDIO</span><span className="studio-header-caption">应用创作工作台</span><div className="studio-header-actions"><button onClick={()=>setGuide(true)}><HelpCircle size={16}/>使用指引</button><a href="/"><ArrowLeft size={15}/>探索社区</a></div></header>
 <div className="studio-layout"><aside className="studio-sidebar"><div className="studio-sidebar-title"><span>我的应用</span><span>{projects.length}/{config?.max_projects??20}</span></div><button className="studio-new" onClick={()=>void newProject()} disabled={busy||active}><Plus size={17}/>新建应用</button><nav aria-label="创作项目">{projects.map(item=><button key={item.id} aria-current={project?.id===item.id?'page':undefined} className={project?.id===item.id?'selected':''} onClick={()=>void openProject(item.id)} disabled={busy}><Folder size={16}/><span>{item.title}<small>{(item.active_run||(item.id===project?.id&&active))?'正在制作':item.version?`v${item.version} · 已保存`:'等待首次创作'}</small></span><ChevronRight size={13}/></button>)}{!projects.length&&<p className="studio-no-projects">你的作品会保存在这里。<br/>先从一个想法开始。</p>}</nav><div className="studio-sidebar-bottom"><span className={`studio-service ${config?.ready?'ready':''}`}/>{booting?'正在连接':config?.ready?'创作服务可用':'创作服务待连接'}<small>同一浏览器，随时回来继续</small></div></aside>
 <main className="studio-main">{booting?<div className="studio-loading" aria-label="正在打开工作台"><span/><span/><span/><p>正在读取你的应用…</p></div>:!session?<div className="studio-error" role="alert">{error}<button onClick={()=>location.reload()}>重新连接</button></div>:!project||!detail?<Welcome studio={studio} onGuide={()=>setGuide(true)}/>:<>
 <div className="studio-project-header"><div><span className="studio-eyebrow">MY APPLICATION</span><h1>{project.title}</h1></div><div><span className="studio-current"><span className="studio-service ready"/>{project.version?`v${project.version} 已保存`:'等待首次创作'}</span><button onClick={()=>setHistory(value=>!value)} aria-expanded={history}><History size={16}/>版本历史<span>{detail.versions.length}</span></button></div></div><FlowSteps ready={project.version>0} active={active}/>
 {history&&<section className="studio-history" aria-label="历史版本"><div><b>每次创作，都会留下版本</b><p>点击查看；恢复版本保留已有应用数据。</p></div><div className="studio-version-list">{detail.versions.length?detail.versions.map(item=><button key={item.version} aria-pressed={version===item.version} disabled={busy||active} onClick={()=>void selectVersion(item.version)}>v{item.version}{item.version===project.version?' · 当前':''}</button>):<span>第一个版本完成后，会保存在这里。</span>}</div></section>}
 {historical&&<div className="studio-history-notice"><History size={16}/><span>正在查看 v{version}，恢复后可继续修改。</span><button disabled={busy||active} onClick={()=>void selectVersion(project.version)}>返回当前 v{project.version}</button><button className="studio-restore" disabled={busy||active} onClick={()=>void restore()}><RotateCcw size={14}/>恢复此版本</button></div>}
 {error&&<div className="studio-error" role="alert">{error}</div>}
 <div className="studio-workspace"><section className="studio-conversation" aria-label="创作对话"><div className="studio-conversation-title"><span><PenSymbol/>需求与修改</span><span>{detail.runs.length}/{config?.max_runs??30}</span></div><div className="studio-messages" ref={messages}>{detail.runs.length?detail.runs.map(run=><article key={run.id}><div className="studio-user-label"><span>你</span>{run.base_version?`修改 v${run.base_version}`:'首次创作'}</div><p className="studio-user-message">{run.prompt}</p><div className={`studio-reply studio-reply-${run.status}`}><span>{run.status==='succeeded'?<Check size={15}/>:run.status==='failed'?<RotateCcw size={15}/>:<span className="studio-service ready"/>}{run.status==='succeeded'?`v${run.result_version} 已保存`:run.status==='running'?'正在制作':run.status==='queued'?'需求已保存':'这次创作未完成'}</span>{run.status==='succeeded'?<p>在右侧直接试用，再告诉我你想改哪里。</p>:run.message&&<p>{run.message}</p>}{run.status==='failed'&&!active&&<button className="studio-text-button" disabled={busy||historical} onClick={()=>setPrompt(run.prompt)}>取回需求，修改后再提交<ArrowRight size={13}/></button>}</div></article>):<div className="studio-empty-chat"><Code2 size={25}/><h2>说说你要做的工具</h2><p>先完成一个具体功能，再逐步完善。每次需求和结果都会留在这里。</p><button onClick={()=>setPrompt(starterIdeas[0].text)}>填入阅读收藏的需求<ArrowRight size={14}/></button></div>}</div>
 {active&&selectedRun&&<CreationStatus run={selectedRun} hasVersion={project.version>0}/>}{selectedRun?.status==='queued'&&<button className="studio-resume" disabled={busy||!config?.ready} onClick={resume}>继续已保存的任务<ArrowRight size={14}/></button>}
 {project.version>0&&!active&&!historical&&<div className="studio-next-actions"><span>下一步：试用后，继续描述修改</span><div><button disabled={busy} onClick={()=>setPrompt('保留已有功能、数据和存储键，增加分类标签与按标签筛选。')}>增加筛选</button><button disabled={busy} onClick={()=>setPrompt('保留全部功能、数据和存储键。优化手机屏幕的排版，增大按钮点击区域。')}>优化手机体验</button></div></div>}
 <form className="studio-composer" onSubmit={event=>{event.preventDefault();void submit()}}><label htmlFor="studio-followup">{project.version?'你想如何继续完善？':'描述应用需要的功能'}</label><textarea id="studio-followup" value={prompt} maxLength={2000} onChange={event=>setPrompt(event.target.value)} onKeyDown={event=>{if((event.metaKey||event.ctrlKey)&&event.key==='Enter'&&!busy&&!active&&!historical&&config?.ready&&prompt.trim().length>=2){event.preventDefault();void submit()}}} placeholder={project.version?'例如：增加已读筛选，保留现有功能和内容。':'例如：做一个能添加、搜索和删除内容的阅读收藏。'}/><div><ComposerTip historical={historical} active={active}/><button type="submit" aria-label={project.version?'提交修改':'开始制作应用'} disabled={busy||active||!config?.ready||prompt.trim().length<2||historical}><ArrowUp size={17}/><span>{busy?'处理中':project.version?'提交修改':'开始制作'}</span></button></div></form></section>
 {version>0?<Preview ref={preview} id={project.id} version={version} locked={active||busy||historical} historical={historical}/>:<PreviewPlaceholder active={active||busy}/>}</div></>}
 </main></div><StudioGuide disabled={busy||active} open={guide} onClose={closeGuide} onStart={()=>void startExample()}/></div>
}

// 小型装饰复用已有图标，保持整个工作台一致的线宽。
function PenSymbol(){return <Code2 size={15}/>}
