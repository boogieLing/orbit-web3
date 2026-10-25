import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "用过再说 UsedIt · 发现值得用的 Web3 应用",
  description: "发现值得尝试的 Web3 应用，记录使用体验，回顾交互记录，整理属于自己的应用精选。",
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
      <head><link rel="preload" as="image" href="/art/portal.webp" /></head>
      <body className="antialiased">{children}</body>
    </html>
  );
}
