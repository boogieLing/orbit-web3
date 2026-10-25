# UsedIt API 合同 v1

前缀 /api/v1。JSON。写请求需要 X-UsedIt-Request: 1，浏览器同源 Cookie；错误 {error:{code,message},request_id}。会话由服务端生成，客户端不传 user_id。时间为 ISO8601 UTC。

## 类型

App: {id,name,category,summary,url,chain_id,contracts:string[],tags:string[],accent,mark,source_url}。
User: {id,display_name,wallet:string|null,preferences:string[],can_publish:boolean}。
Post: {id,kind:"review"|"reference",app_id,title,body,sentiment:"positive"|"neutral"|"negative",tags:string[],author:{name,wallet:string|null},proof_status:"none"|"pending"|"verified"|"recorded"|"unavailable"|"rejected",proof_message,tx_hash,proof_checked_at,created_at,updated_at,revision,helpful_count,bookmarked,helpful,owned,source_url?,reason?,interaction?:InteractionRecord}。
Collection: {id,title,description,instructions,layout:"editorial"|"grid",theme:"ink"|"sage"|"sand",app_ids:string[],post_ids:string[],version,html,created_at,updated_at}。

InteractionRecord: {id,app_id,title,detail,occurred_at}，站内操作摘要；不包含交易哈希。

## 接口

GET /session -> {user,capabilities:{wallet:boolean,proof:boolean,freeform:boolean,experience:boolean},interactions:InteractionRecord[],stats:{reviews,verified,apps}}，无会话时初始化游客。
POST /auth/challenge {address} -> {challenge_id,message}；钱包 personal_sign 签 message。
POST /auth/verify {challenge_id,signature} -> {user} 并替换 Cookie。
POST /auth/logout {} -> {ok:true}。
PATCH /profile {display_name} -> {user}。
GET /apps -> {items:App[]}。
GET /feed?tab=for-you|latest|verified|saved&category=all|identity|exchange|collectibles&q=&cursor= -> {items:Post[],next_cursor:string|null}。
GET /posts/{id} -> {post:Post}。
POST /posts {app_id,title,body,sentiment,tags,tx_hash?}，Idempotency-Key UUID，需 can_publish；默认社区身份可发布 -> 201 {post:Post}。创建时凭据为none，若有tx_hash前端另调verify。
PATCH /posts/{id} {title,body,sentiment,tags,revision} -> {post:Post}，版本冲突409，仅本人。
DELETE /posts/{id} -> {ok:true}，软删除，仅本人。
POST /posts/{id}/verify {record_id}（默认）或 {tx_hash}（钱包路径） -> {post:Post}，仅本人。record_id 必须属于当前应用，成功状态 recorded，与 verified 区分；不查询外部服务。有待确认、拒绝、上游不可用状态都明确返回；不自动重试。
PUT /posts/{id}/bookmark {active:boolean} -> {active}。
PUT /posts/{id}/helpful {active:boolean} -> {active}。
POST /feedback {post_id,kind:"open"|"hide"|"impression",event_id:string,active?:boolean} -> {ok:true}。
PUT /preferences {tags:string[]} -> {user}。
GET /me/posts -> {items:Post[]}。
GET /collections -> {items:Collection[]}。
POST /collections {title,description,instructions,layout,theme,app_ids,post_ids} -> {collection:Collection}。结构化内容编译出单文件HTML；最多各12项。
GET /collections/{id} -> {collection,versions:[{version,created_at}]}，仅本人。
PATCH /collections/{id} 与创建相同另加 version -> {collection}，保存新版本，冲突409。
GET /collections/{id}/versions/{number} -> {collection}，仅本人。
GET /collections/{id}/export -> HTML附件，仅本人。
POST /collections/{id}/compose {instruction} -> 默认执行本地页面规则并保存新版本，支持三组配色/版式及「标题：…」「简介：…」；不识别的要求返回422且不写版本。关闭完整体验后使用外部服务，未配置返回503。

## 界面约束

中文品牌“用过再说”，英文 UsedIt。遵守用户措辞要求。资料笔记保留官方来源；固定内容库用于笔试展示，用户操作正常持久化。独立社区身份无钱包地址，记录关联使用 recorded，不显示为链上核验。
发布、编辑、记录、精选及版本在失败时保留输入并说明原因。精选页有实际 HTML 预览、复制、下载与历史；预览 sandbox 只允许脚本，无同源、无外部请求。本地页面调整仅承诺界面列明的字段，不提供任意应用代码构建。
推荐理由对应实际兴趣与互动信号，余额和收益不进入排序。关闭完整体验后保留原钱包签名、凭据核验和外部页面调整路径。
