import './generated/wasm_exec.js';
import wasm from './generated/core.wasm?module';
let started: Promise<void> | undefined;
type GoRuntime = {importObject: WebAssembly.Imports;run(instance: WebAssembly.Instance): Promise<void>};
type RuntimeGlobal = typeof globalThis & {Go: new()=>GoRuntime;useditHandle(bridge:unknown,input:string,config:string):Promise<string>};
// 分块计数并及早取消超大请求，不能等完整读入内存后才检查。
async function readBody(request: Request): Promise<string|null> {
 if (!request.body) return '';
 // 按块累计，超过上限立即取消读取，避免完整缓冲任意大输入。
 const reader=request.body.getReader(); const chunks:Uint8Array[]=[]; let size=0;
 try {
  while(true){const {done,value}=await reader.read();if(done)break;size+=value.byteLength;if(size>65536){await reader.cancel();return null;}chunks.push(value);}
 }finally{reader.releaseLock();}
 const bytes=new Uint8Array(size);let offset=0;for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.byteLength;}
 return new TextDecoder().decode(bytes);
}
// 每个隔离实例复用一个运行时，只在请求上下文启动。
async function startGo() {
 if (!started) started = (async()=> {
  const Go = (globalThis as RuntimeGlobal).Go;
  const runtime = new Go();
  const instance = await WebAssembly.instantiate(wasm,runtime.importObject);
  void runtime.run(instance);
 })();
 await started;
}
// 每次请求传递独立绑定与身份；全局只复用无业务状态的 Go 运行时。
export async function goRequest(request: Request, env: Cloudflare.Env) {
 try {
  const length=Number(request.headers.get('content-length')??0);
  if(length>65536)return Response.json({error:{code:'body_too_large',message:'请求内容过长'}},{status:413});
  const body=request.method==='GET'?'':await readBody(request);
  if(body===null)return Response.json({error:{code:'body_too_large',message:'请求内容过长'}},{status:413});
  await startGo();
  // 绑定随请求传入，不能缓存到其他用户或其他请求的上下文。
  const bridge={
   async query(sql:string,argsJSON:string){const args=JSON.parse(argsJSON) as (string|number|null)[];return JSON.stringify(await env.DB.prepare(sql).bind(...args).all());},
   async batch(statementsJSON:string){const statements=JSON.parse(statementsJSON) as {SQL:string;Args:(string|number|null)[]}[];return JSON.stringify(await env.DB.batch(statements.map(s=>env.DB.prepare(s.SQL).bind(...(s.Args??[])))));}
  };
  const config=env as Cloudflare.Env & {ETH_RPC_URL?:string;COMPOSE_URL?:string;COMPOSE_API_KEY?:string;COMPOSE_MODEL?:string};
  const input={method:request.method,url:request.url,headers:Object.fromEntries(request.headers),body,platform_id:request.headers.get('oai-authenticated-user-id')??''};
  const result=JSON.parse(await (globalThis as RuntimeGlobal).useditHandle(bridge,JSON.stringify(input),JSON.stringify({RPCURL:config.ETH_RPC_URL??'https://ethereum-rpc.publicnode.com',ComposeURL:config.COMPOSE_URL??'',ComposeKey:config.COMPOSE_API_KEY??'',ComposeModel:config.COMPOSE_MODEL??''}))) as {status:number;headers:Record<string,string>;body:string};
  return new Response(result.body,{status:result.status,headers:result.headers});
 }catch (error) { console.error('go_runtime',error); return Response.json({error:{code:'runtime_unavailable',message:'服务暂不可用，请稍后重试'}},{status:503}); }
}
