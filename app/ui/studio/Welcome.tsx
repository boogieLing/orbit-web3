"use client";
import {ArrowRight,ArrowUpRight,Check,History,Layers,PenLine} from 'lucide-react';
import {starterIdeas} from './Guide';
import type {useStudio} from './useStudio';

type Controller=ReturnType<typeof useStudio>;
// Welcome 给新访客一个可直接填写的需求入口；示例选择不会自动执行调用。
export function Welcome({studio,onGuide}:{studio:Controller;onGuide:()=>void}){
 const {title,setTitle,prompt,setPrompt,submit,busy,config,error}=studio;
 return <section className="studio-welcome"><div className="studio-welcome-copy"><span className="studio-eyebrow">ORBIT STUDIO / CREATE SOMETHING USEFUL</span><h1>你的下一个工具，<br/><em>从一句话开始。</em></h1><p>描述需求，制作一个可以直接使用的网页应用。<br/>先试用，再修改。每个版本，都替你留好。</p></div>
  <div className="studio-launch-grid"><div><form className="studio-start" onSubmit={event=>{event.preventDefault();void submit()}}><div className="studio-form-top"><span><PenLine size={16}/> 描述你的想法</span><small>01 / 开始制作</small></div><label htmlFor="studio-title">应用名称 <span>选填</span></label><input id="studio-title" value={title} maxLength={80} onChange={event=>setTitle(event.target.value)} placeholder="例如：我的阅读收藏"/>
   <label htmlFor="studio-prompt">你希望它能做什么？</label><textarea id="studio-prompt" value={prompt} maxLength={2000} onChange={event=>setPrompt(event.target.value)} onKeyDown={event=>{if((event.metaKey||event.ctrlKey)&&event.key==='Enter'&&!busy&&prompt.trim().length>=2){event.preventDefault();void submit()}}} placeholder="做一个阅读收藏工具，能添加文章、搜索标题、标记已读，刷新后内容还在。" aria-describedby="studio-prompt-help"/><div id="studio-prompt-help" className="studio-input-help">写清功能和风格即可，制作完成后还可以继续改。<span>{prompt.length}/2000</span></div><div className="studio-start-footer"><span><Check size={14}/> 无需配置，直接开始</span><button className="studio-primary" disabled={busy||prompt.trim().length<2} type="submit">{busy?'正在提交…':config?.ready?'开始制作':'保存项目与需求'}<ArrowRight size={17}/></button></div></form>
   {error&&<div className="studio-error" role="alert">{error}</div>}
   <div className="studio-example-heading"><span>还没想好？选一个起点</span><small>点击填入，可自由修改</small></div><div className="studio-ideas">{starterIdeas.map(item=><button key={item.label} onClick={()=>{setTitle(item.label);setPrompt(item.text);document.getElementById('studio-prompt')?.focus()}}><item.icon size={18}/><span><b>{item.label}</b><small>{item.description}</small></span><ArrowUpRight size={15}/></button>)}</div>
  </div><aside className="studio-launch-guide"><div className="studio-orbital-mark" aria-hidden="true"><span/><span/><span/><i/></div><span className="studio-eyebrow">AN IDEA. A WORKING APP.</span><h2>不止看到结果，<br/>还能亲手用它。</h2><ol><li><span>01</span><div><b>写下你要做的事</b><p>从一个具体的小工具开始。</p></div></li><li><span>02</span><div><b>在预览里直接试用</b><p>点击、输入，验证实际功能。</p></div></li><li><span>03</span><div><b>把它改成想要的样子</b><p>继续提需求，恢复版本或导出。</p></div></li></ol><button onClick={onGuide}>查看完整使用指引<ArrowRight size={15}/></button><div className="studio-guide-save"><History size={17}/><p>项目、需求和版本自动保存。<br/>下次用同一浏览器回来继续。</p></div></aside></div>
  {!config?.ready&&<p className="studio-unavailable">创作服务暂不可用。可以先保存项目与需求，稍后回来继续。</p>}<footer className="studio-welcome-foot"><Layers size={14}/><span>制作单页网页工具 · 不需要钱包或资产 · 你的项目独立保存</span></footer>
 </section>
}
