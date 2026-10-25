package community

import "time"

// interactionRecords 返回可选择的站内记录，每次独立构造避免请求间互相修改。
func interactionRecords() []InteractionRecord {
	return []InteractionRecord{
		{ID: "ens-profile", AppID: "ens", Title: "更新个人资料", Detail: "整理头像、个人介绍与常用地址，完成资料预览。", OccurredAt: "2026-10-24T09:20:00Z"},
		{ID: "ens-name", AppID: "ens", Title: "设置主名称", Detail: "选择一个名称作为个人资料入口，检查名称与地址的对应关系。", OccurredAt: "2026-10-23T14:35:00Z"},
		{ID: "uniswap-quote", AppID: "uniswap", Title: "查看兑换报价", Detail: "对比兑换路径、价格影响、网络费用和最低接收数量。", OccurredAt: "2026-10-24T11:10:00Z"},
		{ID: "uniswap-position", AppID: "uniswap", Title: "查看流动性信息", Detail: "阅读价格区间与头寸信息，将关注的问题整理到笔记。", OccurredAt: "2026-10-23T08:40:00Z"},
		{ID: "opensea-item", AppID: "opensea", Title: "浏览作品详情", Detail: "查看作品介绍、所属系列和流转记录，收藏感兴趣的作品线索。", OccurredAt: "2026-10-24T13:30:00Z"},
		{ID: "opensea-series", AppID: "opensea", Title: "整理系列清单", Detail: "对比同一系列的作品信息，整理自己的阅读清单。", OccurredAt: "2026-10-22T16:05:00Z"},
	}
}

type experienceStory struct {
	id, app, author, title, body, sentiment, record string
	tags                                            []string
}

// experienceStories 提供覆盖不同主题、感受与操作记录的固定内容集合。
func experienceStories() []experienceStory {
	// 固定集合覆盖三个应用与不同阅读意图，作者名称不对应钱包账户。
	return []experienceStory{
		{"ens-profile", "ens", "北河", "把零散的地址，收进一张个人名片", "我最开始只是想让朋友更容易找到自己的地址，后来发现，资料页也可以成为一个很轻的个人入口。\n\n这次先整理了头像、个人介绍与常用链接，再逐项检查地址对应关系。比起一次填满，分两次完善更容易检查。\n\n对我来说，真正有用的是信息终于放在了一起。下次需要分享时，不必再从聊天记录里翻找。", "positive", "ens-profile", []string{"链上身份", "域名"}},
		{"ens-name", "ens", "Luna", "设置主名称后，我的地址簿清楚多了", "我把常用地址对应的名称先写进自己的清单，再回到资料页检查显示效果。名称容易记，但最后仍会对照完整地址。\n\n这一步对经常切换入口的人很有用。我的习惯是保留一份简短说明：这个地址用来做什么、哪些页面会用到它。", "positive", "ens-name", []string{"链上身份", "Ethereum"}},
		{"ens-reminder", "ens", "阿棠", "名字选好之后，别忘了管理它的生命周期", "选名称时很容易只关注是否好记，真正整理资料时，我更在意后续如何维护。\n\n我的做法是在自己的备忘录里写清用途和检查日期。即使暂时没有新的操作，也会定期确认资料是否仍然准确。对长期使用来说，维护习惯和第一次设置一样重要。", "neutral", "", []string{"域名", "链上身份"}},
		{"ens-links", "ens", "Rui", "个人资料不用填满，先留下三个常用入口", "第一次整理资料，我想把所有链接都放进去。后来重新看了一遍，发现真正经常用到的只有个人介绍、主页和联系入口。\n\n现在我把资料页当作一张简短的名片：每个链接都说清用途，能不填的就先空着。内容少一点，维护和检查反而更轻松。", "positive", "", []string{"链上身份", "域名"}},
		{"uniswap-quote", "uniswap", "舟行", "报价页里，我会先展开详情再做判断", "刚接触兑换页面时，我的注意力都放在两种代币和数量上。后来开始把报价详情也一起记录。\n\n这次重点看了兑换路径、网络费用和最低接收数量。它们回答的是不同问题，单独对比比只看一个汇总数字更容易理解。\n\n我的笔记里会同时保留当时的阅读目的，这样下次回来就知道自己为什么收藏这条内容。", "positive", "uniswap-quote", []string{"代币兑换", "Ethereum"}},
		{"uniswap-position", "uniswap", "小岚", "看流动性信息前，我先把几个概念分开", "阅读头寸信息时，我先把价格区间、当前状态和费用分别写下来，避免把不同含义的数字混在一起。\n\n对第一次接触这个页面的人来说，先看说明再看数字会更顺畅。我把仍然不理解的地方留在笔记里，等了解之后再补充，而不是凭一个词就做结论。", "neutral", "uniswap-position", []string{"流动性", "Ethereum"}},
		{"uniswap-checklist", "uniswap", "Mika", "给报价阅读留一张自己的检查清单", "我把每次阅读报价时会看的内容整理成了四项：当前网络、两种代币、数量与费用、最低接收数量。\n\n这份清单很短，但回到页面时能让我快速想起要看什么。它不替我决定是否操作，只是帮助我把信息看完整。", "positive", "", []string{"代币兑换", "流动性"}},
		{"uniswap-notes", "uniswap", "青木", "术语很多时，我更喜欢分两次读", "第一次读兑换详情，我发现自己同时记不住所有术语。于是把阅读拆成两次：先确认页面里每个区域的用途，再回来看其中的数字。\n\n这种方式稍微慢一些，但笔记更清楚。我希望入口附近能有更短的术语说明，尤其是刚开始了解这些工具的时候。", "neutral", "", []string{"代币兑换", "Ethereum"}},
		{"opensea-item", "opensea", "南枝", "看一件作品，我会把介绍和流转记录一起读", "吸引我点进去的是作品本身，但决定收藏这条线索之前，我还会看所属系列、作品介绍和活动记录。\n\n这些信息组合起来，才更容易记住它为什么值得继续了解。我把感兴趣的作品按主题整理，回头查找时比只记住图片方便。", "positive", "opensea-item", []string{"数字藏品", "NFT"}},
		{"opensea-series", "opensea", "Kiko", "用自己的主题整理作品，比追着列表刷更有趣", "我给最近关注的作品定了一个小主题，然后把符合主题的条目留在自己的清单里。\n\n每次只补一两条，并写下为什么喜欢。过一段时间再看，这份清单更像一份个人阅读手记，也更容易发现自己的兴趣变化。", "positive", "opensea-series", []string{"数字藏品", "NFT"}},
		{"opensea-details", "opensea", "木子", "名称相似时，先确认自己在看哪一件作品", "浏览作品时，有时会遇到相近的名称。我会回到详情里看系列、网络和编号，而不是只凭标题判断。\n\n整理收藏笔记时，也会把这些定位信息一起留下。以后从其他入口回来，仍能确认是不是之前看的那个条目。", "neutral", "", []string{"数字藏品", "Ethereum"}},
		{"opensea-reading", "opensea", "Echo", "给收藏留一句理由，下次回来就不会忘", "我曾经收藏过不少页面，再打开时却记不起为什么留着。现在会在收藏之后补一句简单的阅读理由。\n\n可能是一段有意思的介绍，也可能是想继续查清楚的问题。把应用和阅读内容放进同一个精选页，浏览记录就更有连续性。", "positive", "", []string{"NFT", "数字藏品"}},
	}
}

// experiencePosts 每次返回独立文章，静态内容不占用个人发布记录与所有权。
func experiencePosts() []Post {
	records := map[string]InteractionRecord{}
	for _, record := range interactionRecords() {
		records[record.ID] = record
	}
	result := []Post{}
	base := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
	for index, story := range experienceStories() {
		created := base.Add(-time.Duration(index) * 2 * time.Hour).Format(time.RFC3339)
		post := Post{ID: "story-" + story.id, Kind: "review", AppID: story.app, Author: Author{Name: story.author}, Title: story.title, Body: story.body, Sentiment: story.sentiment, Tags: story.tags, ProofStatus: "none", CreatedAt: created, UpdatedAt: created, Revision: 1}
		// 记录关联与主网核验使用不同状态，避免展示内容冒充资产凭据。
		if record, ok := records[story.record]; ok {
			post.Interaction = &record
			post.ProofStatus = "recorded"
			post.ProofMessage = "已关联操作记录"
			post.ProofCheckedAt = created
		}
		result = append(result, post)
	}
	return result
}

// libraryPosts 按运行边界提供公共内容，正式钱包环境仍只加载原资料目录。
func (s *Service) libraryPosts() []Post {
	posts := references()
	if s.Config.ExperienceMode {
		posts = append(posts, experiencePosts()...)
	}
	return posts
}
