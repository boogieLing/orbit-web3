export type AppInfo = { id: string; name: string; category: string; summary: string; url: string; chain_id: number; contracts: string[]; tags: string[]; accent: string; mark: string; source_url: string };
export type User = { id: string; display_name: string; wallet: string | null; preferences: string[]; can_publish: boolean };
export type InteractionRecord = { id: string; app_id: string; title: string; detail: string; occurred_at: string };
export type Session = { user: User; capabilities: { wallet: boolean; proof: boolean; freeform: boolean; experience: boolean }; interactions: InteractionRecord[]; stats: { reviews: number; verified: number; apps: number } };
export type Post = {
  id: string; kind: "review" | "reference"; app_id: string; title: string; body: string;
  sentiment: "positive" | "neutral" | "negative"; tags: string[]; author: { name: string; wallet: string | null };
  proof_status: "none" | "pending" | "verified" | "unavailable" | "rejected" | "recorded"; proof_message: string;
  interaction?: InteractionRecord;
  tx_hash: string; proof_checked_at: string; created_at: string; updated_at: string; revision: number;
  helpful_count: number; bookmarked: boolean; helpful: boolean; owned: boolean; source_url?: string; reason?: string;
};
export type Collection = { id: string; title: string; description: string; instructions: string; layout: "editorial" | "grid"; theme: "ink" | "sage" | "sand"; app_ids: string[]; post_ids: string[]; version: number; html: string; created_at: string; updated_at: string };
export type CollectionDetail = { collection: Collection; versions: { version: number; created_at: string }[] };
export type Feed = { items: Post[]; next_cursor: string | null };
export type Tab = "for-you" | "latest" | "verified" | "saved";

export class APIError extends Error {
  constructor(message: string, public status: number, public code = "", public requestID = "") { super(message); this.name = "APIError"; }
}

// 统一同源请求、写入标识、超时与服务端错误，保留请求追踪信息。
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers);
  const method = options.method || "GET";
  if (method !== "GET") { headers.set("Content-Type", "application/json"); headers.set("X-UsedIt-Request", "1"); }
  const timeout = AbortSignal.timeout(path.endsWith("/compose") ? 120000 : 20000);
  const signal = options.signal ? AbortSignal.any([timeout, options.signal]) : timeout;
  const response = await fetch(`/api/v1${path}`, { ...options, headers, signal, credentials: "same-origin", cache: "no-store" });
  const data = await response.json().catch(() => null) as (T & { error?: { code?: string; message?: string }; request_id?: string }) | null;
  if (!response.ok) throw new APIError(data?.error?.message || `请求未能完成（${response.status}）`, response.status, data?.error?.code, data?.request_id);
  if (data === null) throw new APIError("服务返回了无法读取的内容，请稍后重试。", response.status);
  return data as T;
}

// 构建 JSON 写入请求，供业务操作复用相同的请求格式。
export const mutation = (method: string, data: unknown, headers?: HeadersInit): RequestInit => ({ method, headers, body: JSON.stringify(data) });
// 按 Unicode 字符计数，与服务端输入边界保持一致。
export const countChars = (text: string) => Array.from(text).length;
// 按 Unicode 字符截取内容，避免拆开完整字符。
export const trimChars = (text: string, max: number) => Array.from(text).slice(0, max).join("");
// 缩写公开钱包地址或交易哈希，方便在紧凑界面中识别。
export const shortAddress = (address?: string | null) => address ? `${address.slice(0, 6)}…${address.slice(-4)}` : "未连接";
// 将服务端时间显示为中文月日，无效时间返回空文本。
export const dateLabel = (value: string) => { const date = new Date(value); return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString("zh-CN", { month: "long", day: "numeric" }); };
// 将网络、签名取消和业务错误转成可理解且不丢草稿的提示。
export function errorText(error: unknown): string {
  // 优先保留业务错误和主动取消的含义，再对超时及连接故障给出可操作提示。
  if (error instanceof APIError) return error.message;
  if (typeof error === "object" && error && "code" in error && error.code === 4001) return "你取消了钱包操作，草稿仍然保留。";
  if (error instanceof Error && error.name === "TimeoutError") return "请求时间较长，请稍后重试。你的内容仍然保留。";
  if (error instanceof Error && error.message && !error.message.includes("fetch")) return error.message;
  return "连接暂时中断，请检查网络后重试。";
}
// 仅允许网页协议的外部链接，避免不可信内容注入其他协议。
export function externalHref(value?: string): string | undefined {
  try { const url = new URL(value || ""); return ["https:", "http:"].includes(url.protocol) ? url.href : undefined; } catch { return undefined; }
}

type Wallet = { request(args: { method: string; params?: unknown[] }): Promise<unknown> };
declare global { interface Window { ethereum?: Wallet } }

// 请求钱包账户并签署服务端的一次性挑战，再换取社区会话。
export async function signIn(): Promise<User> {
  const wallet = window.ethereum;
  if (!wallet) throw new Error("未检测到浏览器钱包。请启用支持 Ethereum 的钱包扩展，再点击连接；你仍可浏览、收藏和创建精选页。");
  const accounts = await wallet.request({ method: "eth_requestAccounts" });
  const address = Array.isArray(accounts) ? accounts[0] : null;
  if (typeof address !== "string" || !/^0x[0-9a-f]{40}$/i.test(address)) throw new Error("钱包没有返回可用地址，请重新连接。");
  const challenge = await api<{ challenge_id: string; message: string }>("/auth/challenge", mutation("POST", { address }));
  // 精确签署服务端挑战的 UTF-8 字节，不拼接或更改签名内容。
  const messageHex = "0x" + Array.from(new TextEncoder().encode(challenge.message), value => value.toString(16).padStart(2, "0")).join("");
  const signature = await wallet.request({ method: "personal_sign", params: [messageHex, address] });
  if (typeof signature !== "string") throw new Error("未收到钱包签名，请重试。");
  const result = await api<{ user: User }>("/auth/verify", mutation("POST", { challenge_id: challenge.challenge_id, signature }));
  return result.user;
}
