import {execFileSync} from 'node:child_process';
import {mkdirSync,copyFileSync} from 'node:fs';
import path from 'node:path';
// 将相同 Go 核心编译为服务端可加载的模块。
mkdirSync('worker/generated',{recursive:true});
const goroot=execFileSync('go',['env','GOROOT'],{encoding:'utf8'}).trim();
execFileSync('go',['build','-trimpath','-ldflags=-s -w','-o','worker/generated/core.wasm','./cmd/worker'],{stdio:'inherit',env:{...process.env,GOOS:'js',GOARCH:'wasm',CGO_ENABLED:'0'}});
copyFileSync(path.join(goroot,'lib/wasm/wasm_exec.js'),'worker/generated/wasm_exec.js');
