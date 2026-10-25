import { spawn } from 'node:child_process';
import { access } from 'node:fs/promises';
import { constants } from 'node:fs';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../', import.meta.url));
const binary = fileURLToPath(new URL('../bin/verify', import.meta.url));
const maximumBytes = 64 * 1024;
let line = '';
let finished = false;
let timeout;

// restoreTerminal 在退出、取消或启动子进程前恢复终端，不回显已接收内容。
function restoreTerminal() {
  clearTimeout(timeout);
  if (process.stdin.isTTY && process.stdin.isRaw) process.stdin.setRawMode(false);
  process.stdin.pause();
}

// fail 只输出固定原因，不输出输入行、环境变量或子进程参数。
function fail(message, code = 1) {
  if (finished) return;
  finished = true;
  line = '';
  restoreTerminal();
  process.stderr.write(`FAIL ${message}\n`);
  process.exitCode = code;
}

// submit 将唯一输入行经匿名管道交给预先构建的程序，凭据不进入参数或文件。
function submit() {
  if (finished) return;
  finished = true;
  restoreTerminal();
  if (!line.trim()) {
    process.stderr.write('FAIL 缺少验收配置\n');
    process.exitCode = 1;
    return;
  }
  // 不使用 shell 或参数传配置，匿名管道关闭后立即丢弃父进程中的输入字符串。
  const child = spawn(binary, [], { cwd: root, stdio: ['pipe', 'inherit', 'inherit'], shell: false });
  child.on('error', () => {
    process.stderr.write('FAIL 无法启动验收程序\n');
    process.exitCode = 1;
  });
  child.stdin.on('error', () => {
    process.stderr.write('FAIL 验收输入管道已关闭\n');
    process.exitCode = 1;
  });
  child.on('close', (code) => { process.exitCode = code ?? 1; });
  child.stdin.end(line + '\n');
  line = '';
}

// receive 以关闭终端回显的方式读取一行，处理退格、取消和有限输入长度。
function receive(chunk) {
  // 原始终端模式不回显字符；自行处理取消和退格，不能交给行编辑器记录。
  for (const character of chunk) {
    // 首行提交后立即停止，粘贴的后续行不能启动额外验收进程。
    if (finished) return;
    if (character === '\u0003') {
      fail('验收已取消', 130);
      return;
    }
    if (character === '\r' || character === '\n' || character === '\u0004') {
      submit();
      return;
    }
    if (character === '\u007f' || character === '\b') {
      line = [...line].slice(0, -1).join('');
      continue;
    }
    line += character;
    if (Buffer.byteLength(line, 'utf8') > maximumBytes) {
      fail('配置超过大小限制');
      return;
    }
  }
}

// main 只接收标准输入，不读取系统凭据；缺少二进制时提示显式构建命令。
async function main() {
  if (process.argv.length !== 2) {
    fail('请通过标准输入提供配置，不接受命令行参数');
    return;
  }
  try {
    await access(binary, constants.X_OK);
  } catch {
    fail('请先执行 go build -o bin/verify ./cmd/verify');
    return;
  }
  process.on('exit', restoreTerminal);
  process.on('SIGTERM', () => fail('验收已取消', 143));
  process.on('SIGINT', () => fail('验收已取消', 130));
  // 先关闭回显再宣布就绪，确保调用方只能在安全输入阶段提交配置。
  process.stdin.setEncoding('utf8');
  if (process.stdin.isTTY) process.stdin.setRawMode(true);
  process.stdin.on('data', receive);
  process.stdin.on('end', submit);
  process.stdin.on('error', () => fail('标准输入读取失败'));
  timeout = setTimeout(() => fail('等待配置超时'), 120_000);
  process.stdout.write('Ready for verification JSON\n');
  process.stdin.resume();
}

await main();
