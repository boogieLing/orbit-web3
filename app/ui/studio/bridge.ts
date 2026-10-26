const policy = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data: blob:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'";

// 校验来自隔离预览的有界字符串存储，不信任任意消息对象。
export function validState(value: unknown): value is Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  // 按 UTF-8 字节数限制快照，避免多字节字符绕过容量上限。
  const entries = Object.entries(value);
  return entries.length <= 100 && entries.every(([key, item]) => key.length <= 256 && typeof item === 'string')
    && new TextEncoder().encode(JSON.stringify(value)).length <= 32768;
}

// 在应用脚本前注入权限限制与独立存储桥，凭据始终留在父页面。
export function createPreview(html: string, initial: Record<string, string>, channel: string): string {
  const document = new DOMParser().parseFromString(html, 'text/html');
  // 先建立隔离规则与存储桥，再执行应用脚本；不把工作区身份带入预览。
  document.querySelectorAll('base, meta[http-equiv]').forEach(node => node.remove());
  const csp = document.createElement('meta');
  csp.httpEquiv = 'Content-Security-Policy';
  csp.content = policy;
  const bridge = document.createElement('script');
  const seed = JSON.stringify(JSON.stringify(initial)).replace(/</g, '\\u003c');
  const channelJSON = JSON.stringify(channel).replace(/</g, '\\u003c');
  bridge.textContent = `(() => {
    const data = Object.assign(Object.create(null), JSON.parse(${seed}));
    const channel = ${channelJSON};
    // 只将业务数据交给父页面，父页面负责身份与持久化。
    const send = () => parent.postMessage({type:'studio:state',channel,data:{...data}}, '*');
    const storage = {
      get length(){return Object.keys(data).length},
      key(index){return Object.keys(data)[Number(index)] ?? null},
      getItem(key){return Object.hasOwn(data,String(key)) ? data[String(key)] : null},
      // 写入前检查整体容量，不留下部分成功的状态。
      setItem(key,value){
        const next = {...data,[String(key)]:String(value)};
        if(Object.keys(next).length>100 || String(key).length>256 || new TextEncoder().encode(JSON.stringify(next)).length>32768) throw new DOMException('存储空间已满','QuotaExceededError');
        data[String(key)] = String(value); send();
      },
      removeItem(key){delete data[String(key)];send()},
      clear(){for(const key of Object.keys(data)) delete data[key];send()}
    };
    Object.defineProperty(window,'localStorage',{value:storage,writable:false,configurable:false});
    addEventListener('error', event => parent.postMessage({type:'studio:error',channel,message:String(event.message).slice(0,100)},'*'));
    addEventListener('unhandledrejection', () => parent.postMessage({type:'studio:error',channel,message:'应用内有未处理的异步操作，请检查源码或继续修改。'},'*'));
  })();`;
  document.head.insertBefore(bridge, document.head.firstChild);
  document.head.insertBefore(csp, document.head.firstChild);
  return '<!doctype html>\n' + document.documentElement.outerHTML;
}
