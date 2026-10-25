# 目录来源与交互凭据边界

核对日期：2026-10-25。目录限定 Ethereum 主网（chain ID 1），当前只有三个应用与三篇资料笔记。所有链接均已在线读取；官方说明中的产品与部署事实，与本项目自己的核对规则分开陈述。资料笔记没有用户体验作者、虚构点赞或使用次数。

## 官方入口与合约范围

| 应用 / 分类 | 官方入口 | 合约依据 | 本次收录 |
| --- | --- | --- | --- |
| ENS / identity | [ENS App](https://app.ens.domains/) | [ENS 部署文档](https://docs.ens.domains/learn/deployments/) | ETH Registrar Controller (Latest)、Wrapped ETH Registrar Controller |
| Uniswap / exchange | [Uniswap App](https://app.uniswap.org/) | [Uniswap 主网部署文档](https://developers.uniswap.org/docs/protocols/v4/deployments) | Universal Router、2.1.1、2.1.2 |
| OpenSea / collectibles | [OpenSea](https://opensea.io/) | [Seaport 官方仓库部署表](https://github.com/ProjectOpenSea/seaport/blob/main/README.md#deployments) | Seaport 1.6 |

收录地址：

| 应用 | Ethereum 主网直接目标地址 |
| --- | --- |
| ENS | `0x59E16fcCd424Cc24e280Be16E11Bcd56fb0CE547` |
| ENS | `0x253553366Da8546fC250F225fe3d25d0C782303b` |
| Uniswap | `0x66a9893cc07d91d95644aedd05d03f95e1dba8af` |
| Uniswap | `0x4c82d1fbfe28c977cbb58d8c7ff8fcf9f70a2cca` |
| Uniswap | `0x23617e59A5925b2A4Bf75d73ff6711cD0b29De85` |
| OpenSea | `0x0000000000000068F116a894984e2DB1123eB395` |

ENS 官方部署页在核对时说明，主网上 ENSv2 已提前部署，正式切换前注册与解析仍使用 ENSv1；因此本次没有把尚待切换的合约加入范围。Uniswap 的网络部署并不保证同址，本次只取 Ethereum: 1 项。OpenSea 合约地址同时见于 [官方部署说明](https://raw.githubusercontent.com/ProjectOpenSea/seaport/main/docs/Deployment.md) 和 [官方帮助文章](https://support.opensea.io/en/articles/8867119-what-does-a-typed-signature-request-look-like)。后者发布日期为 2024-03-26，当前范围以本次重新读取的官方仓库为准。

## 资料笔记与凭据语义

三篇笔记均明确标注“资料笔记”，正文面向使用者整理具体用途、操作步骤和页面信息，附来源名称、链接和核对时间。没有把这些材料写成个人使用经历，也没有写入固定费用、收益或未经实测的体验结论。合约范围与凭据限制留在本节单独说明。

2026-10-25 重新核对的使用文档与正文对应关系：

| 笔记 | 官方使用来源 | 正文采用的事实 |
| --- | --- | --- |
| 用一个 .eth 名称，整理你的地址与个人资料 | [ENS 注册指南](https://support.ens.domains/en/articles/7882582-register-a-eth-name) | 名称关联地址、头像与社交资料；主名称显示；搜索、选择期限、两步注册；两步之间保留浏览器数据；注册费与网络费分开查看 |
| 确认兑换前，把报价详情里的几个数字看清 | [兑换页字段说明](https://support.uniswap.org/hc/en-us/articles/39862756339341-Uniswap-Web-App-The-Swap-Screen)、[网页兑换步骤](https://support.uniswap.org/hc/en-us/articles/8370549680909-How-to-swap-tokens-with-the-Uniswap-web-app)、[价格影响说明](https://support.uniswap.org/hc/en-us/articles/8671539602317-What-is-price-impact)、[兑换与滑点概念](https://developers.uniswap.org/docs/get-started/concepts/traders/swaps)、[官方入门说明](https://blog.uniswap.org/what-is-uniswap) | 选择网络与代币、填写数量；展开费用、路径、价格影响及最大滑点；查看最低接收数量与成功后的链上记录 |
| 了解一件数字藏品，从介绍与流转记录开始 | [作品页使用指南](https://support.opensea.io/en/articles/8866946-review-your-item-page)、[藏品历史与详情说明](https://support.opensea.io/en/articles/8866993-where-can-i-check-the-history-of-an-nft) | 关于、区块链详情、活动和订单页的用途；网络、藏品编号与合约链接；铸造、转移和销售记录；当前报价与历史成交的区分 |

这些笔记的阅读顺序与提醒是本项目的编辑整理，不声称官方承诺某种使用效果。作品页指南标注发布日期 2026-05-04；藏品历史说明为 2025-06-10；本次均重新读取正文后采用。另保留 [Seaport 官方协议介绍](https://docs.opensea.io/docs/seaport)，只用于下面的凭据范围解释。

凭据只说明：在检查时间，指定 Ethereum 主网交易由当前钱包发送，直接调用目录中的一个合约，回执执行成功，所在区块仍与数据源当前主链一致，并达到 12 个区块确认。第一个确认包含交易所在区块。JSON-RPC 字段含义依据 [Ethereum 官方文档](https://ethereum.org/developers/docs/apis/json-rpc/)。12 是本项目的等待规则，不是 Ethereum 最终性承诺。

这个范围有意保持有限：不验证内部调用、代付交易、其他网络、其他历史合约或业务事件。ENS 的提交步骤不等于注册完成；路由调用不单独证明实际兑换结果；Seaport 可被其他入口使用，不证明访问过 OpenSea 网页。成功交互不证明真人身份、评价质量、藏品真伪或应用安全。结果依赖配置的数据源，并非本项目自行验证共识的轻客户端；后续重组需重新核对。

## 查询与状态合同

调用 `proof.Verify(ctx, client, rpcURL, address, txHash, app)` 时，数据源必须来自服务端配置，地址必须来自已验证的钱包会话，不能由前端指定数据源或用户归属。模块依据应用 ID 重新读取内部目录，调用方修改 App 中的合约切片不会扩大范围。

1. 校验地址与交易哈希后，发出包含 `eth_chainId`、`eth_getTransactionByHash`、`eth_getTransactionReceipt`、`eth_blockNumber` 的固定批次。
2. 成功回执达到 12 个确认时，再发出一次 `eth_getBlockByNumber`，按高度读取主链区块，校验区块号与哈希。
3. 整个过程共用 8 秒上限，父请求更短的时限仍生效；最多 2 次 HTTP POST、5 个 RPC 方法，不自动重试，不在内容列表中逐项核对。

每个响应最多 512 KiB，拒绝跳转、非成功状态、缺少或重复响应 ID、上游错误、截断与异常字段。Go/WASM 另设置标准库 Fetch 专用选项 `redirect=error`、`credentials=omit`，避免传输层提前跟随跳转。日志只记录固定原因码，不记录带密钥的地址、上游正文或用户交易内容。默认仅接受 HTTPS；回环 HTTP 供本地协议测试。

| 状态 | 含义 |
| --- | --- |
| verified | 本次检查的所有条件通过 |
| pending | 未查询到交易、没有回执、确认不足、数据源落后或区块发生变化；可手动重试 |
| rejected | 格式无效、应用不支持、发送者或直接目标不匹配、执行失败 |
| unavailable | 未配置数据源、超时、网络不匹配、响应格式异常或上游失败 |

## 公开数据源实际读取记录

2026-10-25 19:10—19:25（Asia/Shanghai），从本地执行只读 HTTPS POST。请求仅包含 `eth_chainId` 与 `eth_blockNumber`，每个端点最多一次，单次上限 8 秒，未提交交易、签名或钱包连接。

| 端点 | 实际结果 | 耗时 |
| --- | --- | --- |
| `https://ethereum-rpc.publicnode.com` | HTTP 403 | 586 ms |
| `https://eth.llamarpc.com` | HTTP 525 | 1006 ms |
| `https://cloudflare-eth.com` | HTTP 200；chain ID 为 `0x1`，区块高度返回 RPC 错误 `-32046` | 710 ms |
| `https://ethereum.publicnode.com` | HTTP 403 | 825 ms |
| `https://1rpc.io/eth` | HTTP 403 | 820 ms |
| `https://eth.drpc.org` | HTTP 403 | 824 ms |

PublicNode 入口另经 [提供方端点目录](https://publicnode.com/) 核对。上述结果只反映本地网络在该时段的可达性；不能据此认定提供方永久不可用，也不能把 chain ID 单项成功当成完整交易核对成功。当前没有取得公开数据源的完整交易验证成功证据，用户钱包端到端流程也未由这些请求覆盖。部署环境应使用已验证可用的服务端数据源；失败保留明确状态与手动重试入口。

## 本地协议验收

`go test -race ./internal/catalog ./internal/proof` 已通过。测试通过本地 HTTP 服务真实收发 JSON-RPC，覆盖正常双批次、发送者与目标不匹配、失败回执、11/12 确认边界、缺失交易、主链哈希变化、错误网络、异常 ID、大小限制、截断响应、非成功 HTTP、拒绝跳转、跨两批次总超时及目录不可被调用方扩大。测试不访问公网，不代表用户钱包流程或公开数据源已验收。
