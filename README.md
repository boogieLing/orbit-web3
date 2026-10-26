# Orbit · 引力

从发现应用到创作应用。Orbit 保留 Web3 应用推荐与社区体验，并提供独立创作工作台：描述需求，通过 DeepSeek 制作可运行的单页应用，连续修改、保存对话与版本、恢复旧代码。访客使用独立身份；创作服务凭据由服务端配置。

业务逻辑使用 Go，同一领域服务支持原生 HTTP + SQLite，以及服务端 WebAssembly + Cloudflare D1。React 负责展示，Worker 负责请求与存储桥接。

[在线体验](https://usedit-web3.ushouldknowr0.chatgpt.site/) · [源码仓库](https://github.com/boogieLing/orbit-web3) · [完整操作路径](docs/完整体验说明.md)

笔试核心验收从「创作应用」进入；社区推荐、发布、收藏与个人精选继续保留。详见 [应用创作说明](docs/应用创作说明.md)。

## 主要流程

- 进入 [创作工作台](https://usedit-web3.ushouldknowr0.chatgpt.site/studio)，描述需求，预览实际产物，继续增加功能或修改样式。
- 项目、完整需求记录和代码版本持久化；可回退、查看源码、下载独立 HTML。

- 阅读推荐、搜索和分类筛选；收藏、有用反馈、不感兴趣和阅读偏好。
- 使用独立社区身份发布、编辑、删除自己的体验；可从使用笔记开始填写。
- 选择应用对应的交互记录，查看操作摘要并关联到体验。
- 选择应用与文章创建个人精选页，调整版式和主题，运行预览、保存版本并下载独立 HTML。

目录包含 ENS、Uniswap、OpenSea。内置 12 篇使用内容、3 篇带官方来源的资料笔记和 6 条交互记录，供面试展示使用。收藏、反馈、个人内容和精选版本均由实际请求写入数据库，用户之间保持隔离；不预设互动热度。

## 本地启动

需要 Go 1.25+、Node.js 22.13+、Git。所有命令在项目根目录执行。

```sh
npm ci
npm run build
```

首次启动新数据库时应用迁移一次：

```sh
node --import ./scripts/sites-env.mjs ./node_modules/wrangler/bin/wrangler.js d1 execute DB --local --config dist/server/wrangler.json --persist-to .wrangler/state --file drizzle/0000_broken_scarecrow.sql
npm run dev -- --port 18440
```

打开终端显示的本地地址。不要对已经应用该迁移的数据库重复执行迁移。开发与生产包本地预览共享 `.wrangler/state`。

```sh
# 运行已构建的完整 Worker
npm start -- --port 18442

# 单独启动原生 Go API，自动应用尚未执行的迁移
# 该入口提供 API，不包含网页静态文件
go run ./cmd/server -addr 127.0.0.1:18441 -db data/usedit.sqlite
```

新增工作台迁移为 `drizzle/0001_mature_cerise.sql`。使用相同 `d1 execute` 命令对本地旧库追加执行一次，不重复执行旧迁移。生产发布由 Sites 按迁移记录推进。

本地生产 Worker 读取 `dist/server/.dev.vars`，开发服务读取根目录 `.dev.vars`。两者均保持忽略；构建会重建 `dist/`，不要将凭据放进发布归档。

## 验证

```sh
npm run typecheck
npm run test:go
go vet ./cmd/... ./internal/...
go build -o bin/verify ./cmd/verify
node scripts/verify-site.mjs
```

最后一个命令会隐藏读取一行配置。对本地填写 `{"url":"http://127.0.0.1:18440"}`。它使用临时内存密钥进行实际签名和 HTTP 请求，检查收藏、偏好、登录迁移、内容写入、权限、幂等与精选版本；结束时软删除自己创建的验证文章。精选保留在该临时账户下。它不进行链上交易，也不代替浏览器钱包扩展验收。私有站点验收配置可添加原始 `service_token`，程序仅向该站点附加 Bearer 认证头。当前环境中 Go HTTP 客户端访问线上站点会被 Cloudflare 拒绝；线上业务另以标准 Python HTTP 客户端完成检查，详见验收记录。

## 项目布局

| 路径 | 职责 |
| --- | --- |
| `internal/community` | API、身份、内容、反馈、推荐分页、精选版本 |
| `internal/recommend` | 有界、可解释的内存排序 |
| `internal/wallet` | EIP-191 签名与地址恢复 |
| `internal/proof`、`internal/catalog` | 交易核验、受控目录与资料来源 |
| `internal/collectionpage` | 转义、隔离与独立 HTML 编译 |
| `internal/database` | SQLite / D1 适配 |
| `cmd/server`、`cmd/worker` | 原生 / WebAssembly 入口 |
| `cmd/verify` | 实际 HTTP 业务验收 |
| `app` | 中文社区界面 |
| `db`、`drizzle` | 数据结构和不可变迁移 |
| `worker` | Sites 请求边界与 Go 运行时桥接 |

## 配置与边界

- `STUDIO_URL`、`STUDIO_API_KEY`、`STUDIO_MODEL`、`STUDIO_PROVIDER`：独立的应用创作服务。默认接入 DeepSeek Flash，使用 HTTPS `/chat/completions`。密钥只留在服务端；浏览器不接受上游地址或凭据。
- 每用户 20 个项目、每项目 30 次创作、每用户每分钟 2 次、全站每天 50 次；每次最多输出 12,000 token，90 秒超时。失败不自动重试。
- 交付范围为单文件 HTML/CSS/JavaScript 应用；网络、外部依赖与链上交易不在生成应用的执行权限内。

`.env.example` 只列配置名。原生 API 从进程环境读取；本地 Worker 可使用忽略的 `.dev.vars`；线上通过 Sites 环境变量管理保存，密钥不进入源码。

- `USEDIT_EXPERIENCE_MODE`：默认启用站内完整体验；显式设置 `false` 后切换为钱包签名与主网凭据路径。笔试展示保持默认即可。
- `ETH_RPC_URL`：Ethereum HTTP RPC，默认 publicnode。公开服务可能限制访问，此时凭据显示不可用并允许手动重试。
- `COMPOSE_URL`、`COMPOSE_API_KEY`、`COMPOSE_MODEL`：可选的页面调整服务，URL 为完整 HTTPS 接口。仅关闭站内完整体验时使用。默认路径提供本地配色、版式、标题和简介调整，无需此配置。外部服务仅返回受约束的展示配置，不支持任意应用代码。
- 当前钱包签名支持普通外部账户；未支持合约钱包。身份签名不请求交易权限。
- 推荐采用透明规则，不声称已经获得线上推荐效果。阅读与曝光事件有记录，目前进入排序的个性化信号是明确兴趣、收藏应用与排除状态。
- 在线作品与本仓库均公开访问。站点访问策略与应用内身份隔离分别管理，访客只能修改自己的内容和精选。

完整操作路径见 [完整体验说明](docs/完整体验说明.md)。详见 [架构与取舍](docs/架构与取舍.md)、[接口合同](docs/api.md)、[资料与凭据来源](docs/catalog-sources.md)、[验收记录](docs/验收记录.md)、[部署说明](docs/部署说明.md)。
