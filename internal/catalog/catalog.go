// Package catalog 保存经来源核对的有限应用目录；资料笔记不代表用户体验。
package catalog

// App 的合约列表仅用于核对成功的直接交互，不推断具体业务结果。
type App struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Category  string   `json:"category"`
	Summary   string   `json:"summary"`
	URL       string   `json:"url"`
	ChainID   uint64   `json:"chain_id"`
	Contracts []string `json:"contracts"`
	Tags      []string `json:"tags"`
	Accent    string   `json:"accent"`
	Mark      string   `json:"mark"`
	SourceURL string   `json:"source_url"`
}

// Reference 是带出处的资料整理，没有体验作者或互动计数。
type Reference struct {
	ID         string   `json:"id"`
	AppID      string   `json:"app_id"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Tags       []string `json:"tags"`
	SourceURL  string   `json:"source_url"`
	SourceName string   `json:"source_name"`
	CheckedAt  string   `json:"checked_at"`
}

var apps = []App{
	{
		ID: "ens", Name: "ENS", Category: "identity", Summary: "用 .eth 名称管理地址与个人链上资料。", URL: "https://app.ens.domains/", ChainID: 1,
		Contracts: []string{"0x59e16fccd424cc24e280be16e11bcd56fb0ce547", "0x253553366da8546fc250f225fe3d25d0c782303b"},
		Tags:      []string{"链上身份", "域名", "Ethereum"}, Accent: "#5775de", Mark: "E", SourceURL: "https://docs.ens.domains/learn/deployments/",
	},
	{
		ID: "uniswap", Name: "Uniswap", Category: "exchange", Summary: "探索链上代币兑换与流动性工具。", URL: "https://app.uniswap.org/", ChainID: 1,
		Contracts: []string{"0x66a9893cc07d91d95644aedd05d03f95e1dba8af", "0x4c82d1fbfe28c977cbb58d8c7ff8fcf9f70a2cca", "0x23617e59a5925b2a4bf75d73ff6711cd0b29de85"},
		Tags:      []string{"代币兑换", "流动性", "Ethereum"}, Accent: "#d8498c", Mark: "U", SourceURL: "https://developers.uniswap.org/docs/protocols/v4/deployments",
	},
	{
		ID: "opensea", Name: "OpenSea", Category: "collectibles", Summary: "浏览数字藏品，了解基于 Seaport 的订单交互。", URL: "https://opensea.io/", ChainID: 1,
		Contracts: []string{"0x0000000000000068f116a894984e2db1123eb395"},
		Tags:      []string{"数字藏品", "NFT", "Ethereum"}, Accent: "#3b8cc4", Mark: "O", SourceURL: "https://github.com/ProjectOpenSea/seaport/blob/main/README.md#deployments",
	},
}

var references = []Reference{
	{
		ID: "reference-ens", AppID: "ens", Title: "用一个 .eth 名称，整理你的地址与个人资料",
		Body: "资料笔记｜一个 .eth 名称可以关联钱包地址、头像、社交账号和去中心化网站。设为主名称后，支持 ENS 的应用就能用名称显示你的地址，方便分享和识别。\n\n开始时，打开官方应用并连接钱包，搜索想要的名称；确认可注册后选择年限或到期日，也可同时填写个人资料。接下来按页面提示提交注册请求，等待倒计时，再完成第二笔注册交易。两步之间保留原浏览器，不要清除缓存。\n\n提交前分别查看注册费用和网络费用，金额以页面与钱包当时显示为准。完成后回到名称资料页检查地址记录，并记下到期时间，方便后续管理。",
		Tags: []string{"链上身份", "域名", "Ethereum"}, SourceURL: "https://support.ens.domains/en/articles/7882582-register-a-eth-name", SourceName: "ENS 官方注册指南", CheckedAt: "2026-10-25T11:33:24Z",
	},
	{
		ID: "reference-uniswap", AppID: "uniswap", Title: "确认兑换前，把报价详情里的几个数字看清",
		Body: "资料笔记｜兑换页用于选择要卖出和接收的代币，并查看相应数量。先确认当前网络与两种代币，再填写金额；展开报价下方的详情，把代币数量和费用分别看清。\n\n详情里可以查看手续费、网络费用和兑换路径。价格影响描述这笔交易本身对价格的影响；最大滑点则是允许报价到执行之间出现的变动范围。在“最大滑点”旁打开说明，还能看到最低接收数量。\n\n确认前分别核对当前价格、接收数量与最低接收数量，了解自己接受的边界。如果调整金额或设置，重新检查详情和页面提醒，再决定是否继续。页面显示成功后，还可用“View on Explorer”查看链上记录。",
		Tags: []string{"代币兑换", "流动性", "Ethereum"}, SourceURL: "https://support.uniswap.org/hc/en-us/articles/39862756339341-Uniswap-Web-App-The-Swap-Screen", SourceName: "Uniswap 官方兑换页面指南", CheckedAt: "2026-10-25T11:33:24Z",
	},
	{
		ID: "reference-opensea", AppID: "opensea", Title: "了解一件数字藏品，从介绍与流转记录开始",
		Body: "资料笔记｜了解一件数字藏品，可以从作品页的介绍、链上信息和流转记录三个部分开始。先阅读“关于”中的系列说明，弄清作品属于哪个系列；再打开“区块链详情”，查看所在网络、藏品编号和合约链接。\n\n接着查看“活动”页。这里能看到铸造、转移和历史销售等记录，帮助你梳理它从哪里开始、曾由哪些地址持有。作品名称与图片是浏览入口，合约地址和藏品编号则方便你定位同一件藏品、继续查阅公开记录。\n\n如果管理的是自己持有的作品，还可以在“订单”页查看当前报价。先区分作品介绍、历史成交和仍在等待接受的报价，再决定下一步；这些信息各有用途，不宜混在一起比较。",
		Tags: []string{"数字藏品", "NFT", "Ethereum"}, SourceURL: "https://support.opensea.io/en/articles/8866946-review-your-item-page", SourceName: "OpenSea 官方作品页指南", CheckedAt: "2026-10-25T11:33:24Z",
	},
}

// All 返回独立副本，避免调用方修改全局目录与合约范围。
func All() []App {
	result := make([]App, len(apps))
	for index, app := range apps {
		result[index] = cloneApp(app)
	}
	return result
}

// Get 按固定标识读取应用副本，未知标识不会退回其他应用。
func Get(id string) (App, bool) {
	// 只接受完整标识匹配，未知应用不能借用其他应用的合约范围。
	for _, app := range apps {
		if app.ID == id {
			return cloneApp(app), true
		}
	}
	return App{}, false
}

// References 返回带来源的资料笔记副本，调用方修改标签不影响后续读取。
func References() []Reference {
	result := make([]Reference, len(references))
	for index, reference := range references {
		result[index] = reference
		result[index].Tags = append([]string(nil), reference.Tags...)
	}
	return result
}

// cloneApp 独立复制合约与标签切片，保护全局目录的核对范围。
func cloneApp(app App) App {
	app.Contracts = append([]string(nil), app.Contracts...)
	app.Tags = append([]string(nil), app.Tags...)
	return app
}
