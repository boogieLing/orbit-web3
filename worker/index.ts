import { goRequest } from "./go-runtime";
import handler from "vinext/server/fetch-handler";
import { runWithConnectorBinding } from "../lib/connector-context";
import type { ConnectorBinding } from "../lib/connector-contract.mjs";

const worker = {
  fetch(request: Request, env: Cloudflare.Env, ctx: ExecutionContext<{ CONNECTORS?: ConnectorBinding }>) {
    if (new URL(request.url).pathname.startsWith("/api/v1/")) {
      const response = goRequest(request, env);
      // 连接关闭后尽力完成已开始的状态写入；超时任务由项目读取恢复，不自动重复调用。
      if (/\/studio\/projects\/[^/]+\/runs\/[^/]+\/execute$/.test(new URL(request.url).pathname)) ctx.waitUntil(response.then(() => undefined));
      return response;
    }
    let binding = ctx.props?.CONNECTORS;
    // Local preview emulates the same request-scoped capability. This branch and
    // the auxiliary service binding are absent from production builds.
    if (import.meta.env.DEV && !binding && env.CONNECTORS) {
      // 本地连接仅在开发条件内装配，发布包移除此分支。
      const preview = env.CONNECTORS;
      const expiresAt = Date.now() + 60_000;
      binding = {
        // 本地连接上下文过期后不再复用。
        async getContext() {
          if (Date.now() >= expiresAt) return { status: "request_context_expired" };
          return preview.getContext?.() ?? { status: "binding_unavailable" };
        },
        // 调用前再次核验本地连接上下文寿命。
        async invoke(connectorId, actionName, args) {
          if (Date.now() >= expiresAt) {
            return { status: "request_context_expired", message: "This request has expired. Please try again." };
          }
          return preview.invoke(connectorId, actionName, args);
        },
      };
    }
    return runWithConnectorBinding(binding, () => handler.fetch(request, env, ctx));
  },
};

export default worker;
