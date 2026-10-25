# 用过再说 · UsedIt

让真实体验被看见的 Web3 社区。读者通过兴趣、收藏和反馈发现 DApp；作者用钱包签名发布体验，可主动附加一笔链上交互凭据。资料笔记与用户体验分别标注，凭据只证明相应地址的交互事实。

业务逻辑使用 Go，同一领域服务支持原生 HTTP + SQLite，以及服务端 WebAssembly + Cloudflare D1。React 负责展示，Worker 负责请求与存储桥接。

## 主要流程

- 阅读推荐、搜索和分类筛选；收藏、有用反馈、不感兴趣和阅读偏好。
- 钱包签名登录，游客收藏与精选接续；发布、编辑、删除自己的体验。
- 按交易哈希核对 Ethereum 主网发送地址、目标合约、交易回执与确认深度。
- 选择应用与文章创建个人精选页，调整版式和主题，运行预览、保存版本并下载独立 HTML。

首版目录包含 ENS、Uniswap、OpenSea，三篇资料笔记均有官方来源。没有预置用户评价、热度或交易记录。

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

`.env.example` 只列配置名。原生 API 从进程环境读取；本地 Worker 可使用忽略的 `.dev.vars`；线上通过 Sites 环境变量管理保存，密钥不进入源码。

- `ETH_RPC_URL`：Ethereum HTTP RPC，默认 publicnode。公开服务可能限制访问，此时凭据显示不可用并允许手动重试。
- `COMPOSE_URL`、`COMPOSE_API_KEY`、`COMPOSE_MODEL`：可选的页面调整服务，URL 为完整 HTTPS 接口。未配置时界面说明未接入；结构化创建、编辑和预览仍可运行。外部服务仅返回受约束的展示配置，不支持任意应用代码。
- 当前钱包签名支持普通外部账户；未支持合约钱包。身份签名不请求交易权限。
- 推荐采用透明规则，不声称已经获得线上推荐效果。阅读与曝光事件有记录，目前进入排序的个性化信号是明确兴趣、收藏应用与排除状态。
- 新站按默认私有权限发布。用于招聘方访问前需设置相应分享权限；未创建公共 GitHub 仓库。

详见 [架构与取舍](docs/架构与取舍.md)、[接口合同](docs/api.md)、[资料与凭据来源](docs/catalog-sources.md)、[验收记录](docs/验收记录.md)、[部署说明](docs/部署说明.md)。
