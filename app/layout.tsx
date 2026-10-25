import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "用过再说 UsedIt · 让真实体验被看见",
  description: "发现值得尝试的 Web3 应用，记录使用体验，查看交互凭据，整理属于自己的应用精选。",
  other: {
    "codex-preview": "development",
  },
  icons: {
    icon: "/favicon.svg",
    shortcut: "/favicon.svg",
  },
};

// 设置中文页面元数据，并提供社区页面的基础文档结构。
export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN">
      <body className="antialiased">{children}</body>
    </html>
  );
}
