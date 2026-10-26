"use client";
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react';
import { Code2, Download, Monitor, RefreshCw, Smartphone } from 'lucide-react';
import { api, APIError, errorText, mutation } from '../api';
import { createPreview, validState } from './bridge';
import { projectPath } from './types';
import type {AppState,Version} from './types';
export type PreviewHandle={flush:()=>Promise<void>};

// StateWriter 串行推进 revision，快速交互合并保存，冲突后禁止继续覆盖。
class StateWriter {
 pending:Record<string,string>|null=null; saving:Promise<void>|null=null; timer:ReturnType<typeof setTimeout>|undefined; stopped=false;conflict=false;
 constructor(private id:string,private revision:number,private report:(message:string)=>void){}
 // 接收最新状态快照，短暂合并高频输入。
 update(data:Record<string,string>){this.pending=data;if(this.stopped||this.conflict)return;this.report('正在保存应用数据…');clearTimeout(this.timer);this.timer=setTimeout(()=>{void this.flush().catch(()=>undefined)},800)}
 // 切换项目或代码版本前等待实际保存完成。
 flush():Promise<void>{clearTimeout(this.timer);if(this.conflict)return Promise.reject(new Error('其他页面已更新数据，请重新载入。'));if(this.saving)return this.saving;if(!this.pending)return Promise.resolve();this.saving=this.drain().finally(()=>{this.saving=null});return this.saving}
 // 单个 writer 的有界状态刷新顺序执行，不在渲染过程中发起网络请求。
 private async drain(){while(this.pending){const data=this.pending;this.pending=null;try{const saved=await api<AppState>(`${projectPath(this.id)}/state`,mutation('PUT',{data,revision:this.revision}));this.revision=saved.revision}catch(reason){this.pending??=data;this.conflict=reason instanceof APIError&&reason.status===409;this.report(errorText(reason));throw reason}}this.report('应用数据已保存')}
 // 清理定时器并停止接受新写入，已开始请求仍由服务端完成。
 dispose(){this.stopped=true;clearTimeout(this.timer)}
}

// Preview 将不可信代码放入无同源权限的 iframe，并校验每一条存储消息。
export const Preview=forwardRef<PreviewHandle,{id:string;version:number;locked:boolean}>(function Preview({id,version,locked},ref){
 const frame=useRef<HTMLIFrameElement>(null);const writer=useRef<StateWriter|null>(null);const channel=useRef('');
 const [source,setSource]=useState('');const [code,setCode]=useState('');const [error,setError]=useState('');const [status,setStatus]=useState('正在读取应用…');
 const [tab,setTab]=useState<'preview'|'code'>('preview');const [mobile,setMobile]=useState(false);const [refresh,setRefresh]=useState(0);
 useImperativeHandle(ref,()=>({flush:()=>writer.current?.flush()??Promise.resolve()}),[]);
 useEffect(()=>{const controller=new AbortController();let alive=true;setSource('');setError('');setStatus('正在读取应用…');
  Promise.all([api<Version>(`${projectPath(id)}/versions/${version}`,{signal:controller.signal}),api<AppState>(`${projectPath(id)}/state`,{signal:controller.signal})]).then(([saved,state])=>{
   if(!alive)return;channel.current=crypto.randomUUID();writer.current=new StateWriter(id,state.revision,message=>{if(alive)setStatus(message)});setCode(saved.html);setSource(createPreview(saved.html,state.data,channel.current));setStatus('应用数据已保存');
  }).catch(reason=>{if(alive)setError(errorText(reason))});
  return()=>{alive=false;controller.abort();writer.current?.dispose();writer.current=null};
 },[id,version,refresh]);
 useEffect(()=>{
  // 当前 iframe 窗口、opaque origin 与随机通道必须同时匹配。
  const receive=(event:MessageEvent)=>{if(event.source!==frame.current?.contentWindow||event.origin!=='null'||event.data?.channel!==channel.current)return;if(event.data.type==='studio:state'&&validState(event.data.data))writer.current?.update(event.data.data);if(event.data.type==='studio:error'&&typeof event.data.message==='string')setError(event.data.message.slice(0,200))};
  // 浏览器退出前提示尚未完成的数据持久化。
  const closing=(event:BeforeUnloadEvent)=>{if(writer.current?.pending||writer.current?.saving){event.preventDefault();event.returnValue=''}};
  window.addEventListener('message',receive);window.addEventListener('beforeunload',closing);return()=>{window.removeEventListener('message',receive);window.removeEventListener('beforeunload',closing)};
 },[]);
 // 重新载入前先保存，发生冲突时由用户决定是否读取服务端状态。
 const reload=async()=>{try{await writer.current?.flush();setRefresh(value=>value+1)}catch(reason){setError(errorText(reason))}};
 return <section className="studio-preview" aria-label="应用预览">
  <div className="studio-preview-tools"><div><button aria-pressed={tab==='preview'} onClick={()=>setTab('preview')}><Monitor size={15}/>预览</button><button aria-pressed={tab==='code'} onClick={()=>setTab('code')}><Code2 size={15}/>源码</button></div><div><button aria-label={mobile?'切换桌面预览':'切换手机预览'} onClick={()=>setMobile(value=>!value)}>{mobile?<Monitor size={16}/>:<Smartphone size={16}/>}</button><button aria-label="刷新预览" onClick={()=>void reload()}><RefreshCw size={16}/></button><a aria-label="导出当前应用" href={`/api/v1${projectPath(id)}/export`} download><Download size={16}/></a></div></div>
  <div className="studio-browser-bar"><span>● ● ●</span><code>orbit / index.html / v{version}</code><span>隔离预览</span></div>
  {error&&<div className="studio-error" role="alert">{error}<button onClick={()=>{if(!writer.current?.pending||window.confirm('重新读取会舍弃尚未保存的应用操作，是否继续？'))setRefresh(value=>value+1)}}>重新读取保存内容</button></div>}
  <div className={`studio-canvas ${mobile?'studio-canvas-mobile':''}`}>
   {source?<><iframe ref={frame} title="创作应用运行预览" inert={locked} sandbox="allow-scripts" referrerPolicy="no-referrer" srcDoc={source} style={{display:tab==='preview'?'block':'none'}}/><pre hidden={tab!=='code'}><code>{code}</code></pre></>:<div className="studio-preview-empty">正在读取保存的代码…</div>}
  </div><div className="studio-save-status" role="status">{status}<span>代码回退保留应用数据</span></div>
 </section>
});
