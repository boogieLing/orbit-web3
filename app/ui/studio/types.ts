export type Project = { id:string; title:string; version:number; revision:number; active_run:string; created_at:string; updated_at:string };
export type Run = { id:string; prompt:string; status:'queued'|'running'|'succeeded'|'failed'; message:string; provider:string; model:string; base_version:number; result_version:number; created_at:string };
export type Detail = { project:Project; runs:Run[]; versions:{version:number;run_id:string;created_at:string}[] };
export type Version = { version:number; html:string; created_at:string };
export type AppState = { data:Record<string,string>;revision:number };
export type Configuration = {ready:boolean;provider:string;max_projects:number;max_runs:number};
// 项目路径始终编码，避免参数形成其他 API 路由。
export const projectPath=(id:string)=>`/studio/projects/${encodeURIComponent(id)}`;
