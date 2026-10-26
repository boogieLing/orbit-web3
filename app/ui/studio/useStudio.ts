"use client";
import {useEffect,useRef,useState} from 'react';
import {api,errorText,mutation} from '../api';
import type {Session} from '../api';
import type {PreviewHandle} from './Preview';
import {projectPath} from './types';
import type {Configuration,Detail,Project} from './types';

// useStudio 统一管理持久任务与版本，界面和新手指引不直接触发上游请求。
export function useStudio(){
 const [session,setSession]=useState<Session|null>(null);const [config,setConfig]=useState<Configuration|null>(null);const [projects,setProjects]=useState<Project[]>([]);const [detail,setDetail]=useState<Detail|null>(null);
 const [prompt,setPrompt]=useState('');const [title,setTitle]=useState('');const [error,setError]=useState('');const [busy,setBusy]=useState(false);const [booting,setBooting]=useState(true);const [history,setHistory]=useState(false);const [version,setVersion]=useState(0);
 const messages=useRef<HTMLDivElement>(null);const preview=useRef<PreviewHandle>(null);const currentID=useRef('');const operation=useRef(false);const mounted=useRef(true);const generation=useRef(0);
 const project=detail?.project;const active=!!project?.active_run;const selectedRun=detail?.runs.find(run=>run.id===project?.active_run);
 // 先建立唯一身份，再读取依赖会话的数据；启动请求失败可通过重新加载恢复。
 useEffect(()=>{const controller=new AbortController();mounted.current=true;api<Session>('/session',{signal:controller.signal}).then(async value=>{
  const [settings,list]=await Promise.all([api<Configuration>('/studio/config',{signal:controller.signal}),api<{items:Project[]}>('/studio/projects',{signal:controller.signal})]);if(controller.signal.aborted)return;setPrompt(localStorage.getItem(`orbit:studio:draft:${value.user.id}:new`)??'');setSession(value);setConfig(settings);setProjects(list.items);
  const saved=localStorage.getItem(`orbit:studio:current:${value.user.id}`);if(saved&&list.items.some(item=>item.id===saved)){await openProject(saved,value.user.id)}
 }).catch(reason=>{if(!controller.signal.aborted)setError(errorText(reason))}).finally(()=>{if(!controller.signal.aborted)setBooting(false)});return()=>{mounted.current=false;controller.abort()};},[]);
 // 草稿按身份和项目保存，未提交内容不会因刷新或服务暂不可用而丢失。
 useEffect(()=>{if(session&&!booting)localStorage.setItem(`orbit:studio:draft:${session.user.id}:${project?.id??'new'}`,prompt)},[prompt,project?.id,session,booting]);
 useEffect(()=>{if(!active||!project)return;let alive=true;const id=project.id;const timer=setInterval(()=>{api<Detail>(projectPath(id)).then(value=>{if(alive&&currentID.current===id){setDetail(value);if(!value.project.active_run)setVersion(value.project.version)}}).catch(reason=>{if(alive)setError(errorText(reason))})},2500);return()=>{alive=false;clearInterval(timer)}},[active,project?.id]);
 // 新任务与完成状态到达时只滚动对话容器，避免手机整页跳动。
 useEffect(()=>{const list=messages.current;if(list)list.scrollTop=list.scrollHeight},[detail?.runs.length,detail?.runs.at(-1)?.status]);
 // 切换项目时刷新权威数据，并丢弃先前加载的过期响应。
 async function openProject(id:string,userID=session?.user.id){
  try{await preview.current?.flush();const request=++generation.current;setBusy(true);const result=await api<Detail>(projectPath(id));if(!mounted.current||request!==generation.current)return;currentID.current=id;setDetail(result);setVersion(result.project.version);setHistory(false);setError('');setPrompt(localStorage.getItem(`orbit:studio:draft:${userID}:${id}`)??'');if(userID)localStorage.setItem(`orbit:studio:current:${userID}`,id)}catch(reason){if(mounted.current)setError(errorText(reason))}finally{if(mounted.current)setBusy(false)}
 }
 // 回到新项目入口前保存应用数据，原项目和历史都保留在列表中。
 async function newProject(){if(busy||active)return false;try{await preview.current?.flush();generation.current++;currentID.current='';setDetail(null);setVersion(0);setTitle('');setPrompt(localStorage.getItem(`orbit:studio:draft:${session?.user.id}:new`)??'');setError('');if(session)localStorage.removeItem(`orbit:studio:current:${session.user.id}`);return true}catch(reason){setError(errorText(reason));return false}}
 // 更新列表只读项目元信息，避免在列表中批量传输历史代码。
 async function refreshList(){const result=await api<{items:Project[]}>('/studio/projects');if(mounted.current)setProjects(result.items)}
 // 执行已有持久任务，重复触发由服务端原子领取保护。
 async function execute(id:string,runID:string){
  try{const result=await api<Detail>(`${projectPath(id)}/runs/${encodeURIComponent(runID)}/execute`,mutation('POST',{}));if(mounted.current&&currentID.current===id){setDetail(result);setVersion(result.project.version)}}catch(reason){if(mounted.current)setError(errorText(reason));const result=await api<Detail>(projectPath(id)).catch(()=>null);if(result&&mounted.current&&currentID.current===id)setDetail(result)}finally{if(mounted.current){await refreshList().catch(()=>undefined);setBusy(false)}operation.current=false}
 }
 // 先保存项目再创建任务，任务成功接收后才清除需求输入。
 async function submit(){
  if(operation.current||!session||active||(project&&version!==project.version)||prompt.trim().length<2)return;operation.current=true;setBusy(true);setError('');
  // 先完成应用数据写入，再推进代码创作，避免新预览覆盖未保存交互。
  try{await preview.current?.flush();let selected=project;
   if(!selected){const id=crypto.randomUUID();const result=await api<{project:Project}>('/studio/projects',mutation('POST',{id,title:(title.trim()||Array.from(prompt.trim()).slice(0,36).join('')||'我的新应用')}));selected=result.project;currentID.current=selected.id;setDetail({project:selected,runs:[],versions:[]});setVersion(0);localStorage.setItem(`orbit:studio:current:${session.user.id}`,selected.id);localStorage.setItem(`orbit:studio:draft:${session.user.id}:${selected.id}`,prompt);await refreshList()}
   if(!config?.ready){setError('项目已保存。创作服务连接后，可继续提交这份需求。');return}
   const queued=await api<{run:{id:string}}>(`${projectPath(selected.id)}/runs`,mutation('POST',{prompt,revision:selected.revision,request_key:crypto.randomUUID()}));const result=await api<Detail>(projectPath(selected.id));if(!mounted.current)return;setDetail(result);setPrompt('');await execute(selected.id,queued.run.id);
  }catch(reason){if(mounted.current)setError(errorText(reason))}finally{operation.current=false;if(mounted.current)setBusy(false)}
 }
 // 代码恢复使用项目 revision 校验，保留独立的应用业务状态。
 async function restore(){if(!project||busy)return;setBusy(true);try{await preview.current?.flush();await api(`${projectPath(project.id)}/restore`,mutation('POST',{version,revision:project.revision}));await openProject(project.id);await refreshList()}catch(reason){setError(errorText(reason))}finally{setBusy(false)}}

 // 查看历史前保存已有数据；运行中保持当前预览，避免切换造成误解。
 async function selectVersion(next:number){if(busy||active)return;try{await preview.current?.flush();setVersion(next)}catch(reason){setError(errorText(reason))}}
 // 只恢复已有排队任务，原子执行协议保证不会重复创建新任务。
 function resume(){if(project&&selectedRun&&!operation.current){operation.current=true;setBusy(true);void execute(project.id,selectedRun.id)}}
 return {session,config,projects,detail,project,active,selectedRun,prompt,setPrompt,title,setTitle,error,busy,booting,history,setHistory,version,messages,preview,openProject,newProject,submit,restore,selectVersion,resume};
}
